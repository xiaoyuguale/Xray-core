package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/xtls/xray-core/common/cmdarg"
	"github.com/xtls/xray-core/common/errors"
	clog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/common/platform"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/main/commands/base"
)

var cmdRun = &base.Command{
	UsageLine: "{{.Exec}} run [-c config.json] [-confdir dir]",
	Short:     "Run Xray with config, the default command",
	Long: `
Run Xray with config, the default command.

The -config=file, -c=file flags set the config files for 
Xray. Multiple assign is accepted.

The -confdir=dir flag sets a dir with multiple json config

The -format=json flag sets the format of config files. 
Default "auto".

The -test flag tells Xray to test config files only, 
without launching the server.

The -dump flag tells Xray to print the merged config.
	`,
}

func init() {
	cmdRun.Run = executeRun // break init loop
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

var (
	// cmdarg.Arg是一个string类型的切片，用来实现对多个config参数的支持，xray run -c abc -c xyz
	// flag包默认只能解析基本类型的参数，int，string，bool等，但是提供了flag.Var函数用于对自定义类型进行解析
	// flag.Var的value参数是一个flag.Value接口类型，有String和Set方法（实现了接口的方法，就是实现了这个接口）
	// cmdarg.Arg实现了String和Set方法，用于把所有解析到的config参数存储到configFiles这个string类型的切片（跳转common/cmdarg/cmdarg.go查看）
	configFiles cmdarg.Arg // "Config file for Xray.", the option is customed type, parse in main
	configDir   string
	dump        = cmdRun.Flag.Bool("dump", false, "Dump merged config only, without launching Xray server.")
	test        = cmdRun.Flag.Bool("test", false, "Test config file only, without launching Xray server.")
	format      = cmdRun.Flag.String("format", "auto", "Format of input file.")

	/* We have to do this here because Golang's Test will also need to parse flag, before
	 * main func in this file is run.
	 */
	_ = func() bool {
		cmdRun.Flag.Var(&configFiles, "config", "Config path for Xray.")
		cmdRun.Flag.Var(&configFiles, "c", "Short alias of -config")
		cmdRun.Flag.StringVar(&configDir, "confdir", "", "A dir with multiple json config")

		return true
	}()
)

func executeRun(cmd *base.Command, args []string) {
	// -dump参数表示合并所有配置并打印，然后退出程序
	if *dump {
		// 这里Logger的一系列操作没看懂，可能是创建一个为dumpConfig服务的Logger对象？（跳转common/log/logger.go查看）
		clog.ReplaceWithSeverityLogger(clog.Severity_Warning)
		// 查看dumpConfig的定义
		errCode := dumpConfig()
		os.Exit(errCode)
	}

	printVersion()
	server, err := startXray()
	if err != nil {
		fmt.Println("Failed to start:", err)
		// Configuration error. Exit with a special value to prevent systemd from restarting.
		os.Exit(23)
	}

	// test参数表示仅检查config.json格式，不启动server。实际上配置格式的检查在startXray里面执行
	if *test {
		fmt.Println("Configuration OK.")
		os.Exit(0)
	}

	if err := server.Start(); err != nil {
		fmt.Println("Failed to start:", err)
		os.Exit(-1)
	}
	defer server.Close()

	/*
		conf.FileCache = nil
		conf.IPCache = nil
		conf.SiteCache = nil
	*/

	// Explicitly triggering GC to remove garbage from config loading.
	runtime.GC()
	debug.FreeOSMemory()

	{
		osSignals := make(chan os.Signal, 1)
		signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
		<-osSignals
	}
}

func dumpConfig() int {
	// 获取所有config文件，查看getConfigFilePath的定义
	files := getConfigFilePath(false)
	if config, err := core.GetMergedConfig(files); err != nil {
		fmt.Println(err)
		time.Sleep(1 * time.Second)
		return 23
	} else {
		fmt.Print(config)
	}
	return 0
}

func fileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && !info.IsDir()
}

func dirExists(file string) bool {
	if file == "" {
		return false
	}
	info, err := os.Stat(file)
	return err == nil && info.IsDir()
}

func getRegepxByFormat() string {
	switch strings.ToLower(*format) {
	case "json":
		return `^.+\.(json|jsonc)$`
	case "toml":
		return `^.+\.toml$`
	case "yaml", "yml":
		return `^.+\.(yaml|yml)$`
	default:
		return `^.+\.(json|jsonc|toml|yaml|yml)$`
	}
}

func readConfDir(dirPath string) {
	confs, err := os.ReadDir(dirPath)
	if err != nil {
		log.Fatalln(err)
	}
	for _, f := range confs {
		// getRegepxByFormat根据-format参数返回一个正则表达式，用来匹配符合格式的文件
		matched, err := regexp.MatchString(getRegepxByFormat(), f.Name())
		if err != nil {
			log.Fatalln(err)
		}
		if matched {
			// 把符合的文件名保存到变量configFiles（cmdarg.Arg类型，实际上是字符串切片）
			configFiles.Set(path.Join(dirPath, f.Name()))
		}
	}
}

func getConfigFilePath(verbose bool) cmdarg.Arg {
	// 判断configDir是否存在，存在的话，是否是目录
	if dirExists(configDir) {
		if verbose {
			log.Println("Using confdir from arg:", configDir)
		}
		// 如果是目录，读取目录内指定format的文件名，保存到变量configFiles
		readConfDir(configDir)
	// 如果configDir不存在或者不是目录，就从环境变量中获取config路径，并且判断是否目录
	} else if envConfDir := platform.GetConfDirPath(); dirExists(envConfDir) {
		if verbose {
			log.Println("Using confdir from env:", envConfDir)
		}
		// 如果是目录，读取目录内指定format的文件名，保存到变量configFiles
		readConfDir(envConfDir)
	}

	// 如果configFiles有多个，直接返回
	if len(configFiles) > 0 {
		return configFiles
	}

	if workingDir, err := os.Getwd(); err == nil {
		suffixes := []string{".json", ".jsonc", ".toml", ".yaml", ".yml"}
		for _, suffix := range suffixes {
			configFile := filepath.Join(workingDir, "config"+suffix)
			if fileExists(configFile) {
				if verbose {
					log.Println("Using default config: ", configFile)
				}
				return cmdarg.Arg{configFile}
			}
		}
	}

	if configFile := platform.GetConfigurationPath(); fileExists(configFile) {
		if verbose {
			log.Println("Using config from env: ", configFile)
		}
		return cmdarg.Arg{configFile}
	}

	if verbose {
		log.Println("Using config from STDIN")
	}
	return cmdarg.Arg{"stdin:"}
}

func getConfigFormat() string {
	f := core.GetFormatByExtension(*format)
	if f == "" {
		f = "auto"
	}
	return f
}

func startXray() (core.Server, error) {
	configFiles := getConfigFilePath(true)

	// config, err := core.LoadConfig(getConfigFormat(), configFiles[0], configFiles)

	c, err := core.LoadConfig(getConfigFormat(), configFiles)
	if err != nil {
		return nil, errors.New("failed to load config files: [", configFiles.String(), "]").Base(err)
	}

	server, err := core.New(c)
	if err != nil {
		return nil, errors.New("failed to create server").Base(err)
	}

	return server, nil
}
