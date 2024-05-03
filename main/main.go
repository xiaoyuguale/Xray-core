package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/xtls/xray-core/main/commands/base"
	_ "github.com/xtls/xray-core/main/distro/all" // 下划线表示仅执行package的init函数
)

func main() {
	os.Args = getArgsV4Compatible()

	// base.RootCommand是*base.Command类型的全局变量，定义在main/commands/base/root.go中
	// 导入main/commands/base时，main/commands/base/root.go的init函数会实例化一个base.Command类型的结构体，把结构体指针赋值给base.RootCommand
	// *base.Command.Commands是一个*base.Command类型变量的切片（子命令的实现方式？）
	// 在main/distro/all中会导入了main/commands/all，而main/commands/all/commands.go的init函数会为base.RootCommand.Commands追加5个命令（api，tls，uuid，x25519，wg，追加之前Commands列表为空）
	base.RootCommand.Long = "Xray is a platform for building proxies."
	// 这里再在切片开头增加run和version命令
	// 每个命令有对应的init函数，会对命令结构体的内容进行填充
	// 最底层的命令会有对应的execute函数，在init阶段会赋值给命令结构体的Run字段，不是最底层命令，Commands切片填充支持的子命令
	// 主要分析下run命令（跳转main/run.go查看）
	base.RootCommand.Commands = append(
		[]*base.Command{
			cmdRun,
			cmdVersion,
		},
		base.RootCommand.Commands...,
	)
	base.Execute()
}

func getArgsV4Compatible() []string {
	// os.Args长度为1时，表示命令行仅有程序xray，设置默认command为run，并返回
	if len(os.Args) == 1 {
		fmt.Println("提示：命令行仅有程序xray，设置默认command为run，返回xray run")
		return []string{os.Args[0], "run"}
	}
	// os.Args[1]不以-开头，表示xray后指定了command，例如xray uuid，直接返回
	if os.Args[1][0] != '-' {
		fmt.Println("提示：已为程序xray指定了command，直接返回")
		return os.Args
	}
	version := false
	// CommandLine在Parse()错误时会程序会直接退出，这里不适用
	// 这里用NewFlagSet新建一个flag，用来解析参数version
	// flag.ContinueOnError表示解析出错时会返回错误，程序不会直接退出
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.BoolVar(&version, "version", false, "")
	// parse silently, no usage, no error output
	fs.Usage = func() {}
	fs.SetOutput(&null{})
	// NewFlagSet的Parse需要手动传入参数，CommandLine的Parse自动传入了os.Args[1:]
	err := fs.Parse(os.Args[1:])
	// ErrHelp错误表示解析到-h或者-help参数但是未定义该flag
	// 因为Parse()在遇到第一个解析错误时就会返回，想要进入下面的代码，命令行需要是先解析到-h时出错的形式：
	// 1. xray -h
	// 2. xray -version -h（因为已经定义了version参数，所以-version可以被正确解析）
	// xray -run -h这样的形式会先返回解析run的错误，所以就没法进入下面的代码
	if err == flag.ErrHelp {
		// fmt.Println("DEPRECATED: -h, WILL BE REMOVED IN V5.")
		// fmt.Println("PLEASE USE: xray help")
		// fmt.Println()
		fmt.Println("提示：检测到ErrHelp错误：解析到-h或者-help参数，但是未定义该flag，返回xray help")
		return []string{os.Args[0], "help"}
	}
	// 解析到version为true，会进入到下面的代码，命令行需要是先解析到-version的形式：
	// 1. xray -version
	// 2. xray -version -run（虽然解析到-run会返回错误，但是错误不是ErrHelp，上面的判断不通过，此时version为true，可以进入下面的代码）
	// xray -run -version这样的形式会先返回解析run的错误，此时version还未解析为true，所以就没法进入下面的代码
	// xray -version -h这样的形式虽然也能解析到version为true，但是这个时候Parse()没有返回，只有在解析到-h时Parse()才会返回，返回值是Errhelp，会进入上面的代码
	if version {
		// fmt.Println("DEPRECATED: -version, WILL BE REMOVED IN V5.")
		// fmt.Println("PLEASE USE: xray version")
		// fmt.Println()
		fmt.Println("提示：解析到-version or --version or -version=true，返回xray version")
		return []string{os.Args[0], "version"}
	}
	// fmt.Println("COMPATIBLE MODE, DEPRECATED.")
	// fmt.Println("PLEASE USE: xray run [arguments] INSTEAD.")
	// fmt.Println()
	// 除了上面的情况，其他情况都会执行下面的代码，例如：
	// 1. xray -run -h
	// 2. xray -run -version
	fmt.Println("提示：命令行参数格式错误，设置默认command为run，返回其他命令行参数")
	return append([]string{os.Args[0], "run"}, os.Args[1:]...)
}

// 定义空结构体
type null struct{}

// SetOutPut需要传入一个io.Writer的接口，默认的output是os.Stderr
// io.Writer接口只有一个方法Write，这里对null结构体定义Write方法，就实现了这个io.Writer接口，用来屏蔽Parse()的出错信息
func (n *null) Write(p []byte) (int, error) {
	return len(p), nil
}
