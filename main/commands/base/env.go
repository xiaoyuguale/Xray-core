package base

// CommandEnvHolder is a struct holds the environment info of commands
type CommandEnvHolder struct {
	// Executable name of current binary
	Exec string
	// commands column width of current command
	CommandsWidth int
}

// CommandEnv holds the environment info of commands
var CommandEnv CommandEnvHolder

func init() {
	/* 由于上游修改，需要重新分析，查看https://github.com/XTLS/Xray-core/commit/030c9efc8ce914bf2fcf97aa7cc91282397fa590
	// os.Executable返回当前执行程序的绝对路径
	exec, err := os.Executable()
	if err != nil {
		return
	}
	// path.Base返回参数路径的最后一部分，这里用来获取当前执行程序的文件名
	// 如果参数是空字符串，返回"."
	// 如果参数是"/"，返回"/"
	// 其他情况，去掉末尾的"/"，返回最后一个"/"后面的部分
	CommandEnv.Exec = path.Base(exec)
	// 这里为什么获取Base后又重新设定为xray？ */
	/*
		exec, err := os.Executable()
		if err != nil {
			return
		}
		CommandEnv.Exec = path.Base(exec)
	*/
	CommandEnv.Exec = "xray"
}
