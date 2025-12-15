package handler

import "os"

// 便于在 unit test 或不同运行方式下替换
func osArgs() []string {
	return os.Args
}

func osEnviron() []string {
	return os.Environ()
}
