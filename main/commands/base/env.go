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
	/*
		exec, err := os.Executable()
		if err != nil {
			return
		}
		CommandEnv.Exec = path.Base(exec)
	*/
	// 这部分代码和下面直接赋值"xray"的代码重复了，在030c9efc8ce914bf2fcf97aa7cc91282397fa590中移除了
	// https://github.com/XTLS/Xray-core/commit/030c9efc8ce914bf2fcf97aa7cc91282397fa590
	CommandEnv.Exec = "xray"
}
