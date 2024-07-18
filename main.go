package main

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	copySuffix  = "_copy"
	filtersName = "filters.list"
)

var (
	user32          = syscall.MustLoadDLL("user32.dll")
	procMessageBoxW = user32.MustFindProc("MessageBoxW")

	xDir        string
	origPath    string
	copyPath    string
	filtersPath string
	filters     []string
)

func init() {
	// 获取可执行文件路径
	d, err := os.Executable()
	if err != nil {
		msgBox(0, "panic！无法获取可执行文件路径", err.Error(), 0)
		panic(err)
	}
	xDir = filepath.Dir(d)

	// 读取过滤器
	filtersPath = filepath.Join(xDir, filtersName)
	filtersF, err := os.Open(filtersPath)
	if err != nil {
		msgBox(0, fmt.Sprintf("panic！找不到%s", filtersPath), err.Error(), 0)
		panic(err)
	}
	defer filtersF.Close()
	scanner := bufio.NewScanner(filtersF)
	for scanner.Scan() {
		filters = append(filters, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		msgBox(0, "panic！过滤器获取失败", err.Error(), 0)
		panic(err)
	}
	if len(filters) == 0 {
		msgBox(0, "panic！过滤器为空", fmt.Sprintf("你都保护了什么啊\n请检查%s", filtersPath), 0)
		os.Exit(1)
	}

	// 获取目标程序文件名
	// 0: self
	// 1: debug destination
	origProg := filepath.Base(os.Args[1])
	origName := strings.TrimSuffix(origProg, ".exe")
	// 拼接绝对路径
	origPath = filepath.Join(xDir, origName+".exe")
	fmt.Printf("目标程序:\n%s\n", origPath)
	copyPath = filepath.Join(xDir, origName+copySuffix+".exe")
}

func main() {
	// 除去自身和debug目标
	args := os.Args[2:]

	fmt.Println("\n过滤列表:")
	for _, filter := range filters {
		fmt.Println(filter)
	}
	fmt.Println("\n调用方传参:")
	for _, a := range args {
		fmt.Println(a)
	}

	// 匹配过滤
	foundFilteredArgs := make([]string, 0)
	for _, arg := range args {
		for _, filter := range filters {
			if arg == filter {
				foundFilteredArgs = append(foundFilteredArgs, arg)
			}
		}
	}

	if len(foundFilteredArgs) == 0 {
		initAndRun(args)
	} else {
		ppid := syscall.Getppid()
		list := strings.Join(foundFilteredArgs, "\n")
		title := "发现黑名单参数:"
		msg := fmt.Sprintf("%s\n\n调用方信息:\nPID: %d\n名称: %s", list, ppid, getProcessName(ppid))
		fmt.Printf("\n%s\n%s\n", title, msg)
		msgBox(0, title, msg, 0)
	}
}

// initAndRun 初始化并运行
func initAndRun(args []string) {
	if err := checkCopy(); err != nil {
		fmt.Println("初始化失败:\n", err)
		msgBox(0, "初始化失败", err.Error(), 0)
		return
	}

	cmd := exec.Command(copyPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Println("启动失败:\n", err)
		msgBox(0, "启动失败", err.Error(), 0)
	}
}

// checkCopy 检查副本
func checkCopy() error {
	copyFi, err := os.Stat(copyPath)
	if err != nil {
		fmt.Printf("%s不存在\n", copyPath)
		return copyWithModTime()
	}

	origFi, err := os.Stat(origPath)
	if err != nil {
		return fmt.Errorf("找不到原%s: %s", filepath.Base(origPath), err.Error())
	}

	// 比较修改时间
	if origFi.ModTime() == copyFi.ModTime() {
		return nil
	} else {
		// 比较MD5
		isSame, err := compareMD5()
		if err != nil {
			return err
		}
		if !isSame {
			fmt.Printf("%s已变更\n", filepath.Base(origPath))
			return copyWithModTime()
		}
	}

	return nil
}

// copyWithModTime 创建副本并尝试编辑修改时间
func copyWithModTime() error {
	origF, err := os.Open(origPath)
	if err != nil {
		return err
	}
	defer origF.Close()

	origFi, err := origF.Stat()
	if err != nil {
		return err
	}

	destF, err := os.Create(copyPath)
	if err != nil {
		return err
	}
	defer destF.Close()

	_, err = io.Copy(destF, origF)
	if err != nil {
		return err
	}

	// Windows下无法赋上修改时间
	err = os.Chtimes(copyPath, time.Now(), origFi.ModTime())
	if err != nil {
		return err
	}

	return nil
}

// compareMD5 对比MD5
func compareMD5() (bool, error) {
	origMD5, err := calcMD5(origPath)
	if err != nil {
		return false, err
	}

	copyMD5, err := calcMD5(copyPath)
	if err != nil {
		return false, err
	}

	return string(origMD5) == string(copyMD5), nil
}

// calcMD5 计算MD5
func calcMD5(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	hasher := md5.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

// msgBox 弹窗
func msgBox(hwnd uintptr, title, caption string, flags uint) int {
	c, _ := syscall.UTF16PtrFromString(caption)
	t, _ := syscall.UTF16PtrFromString(title)
	ret, _, _ := procMessageBoxW.Call(
		hwnd,
		uintptr(unsafe.Pointer(c)),
		uintptr(unsafe.Pointer(t)),
		uintptr(flags),
	)
	return int(ret)
}

// getProcessName 获取进程名
func getProcessName(pid int) string {
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid)).Output()
	if err != nil {
		return "<未知>"
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) < 4 {
		return "<未知>"
	}
	fields := strings.Fields(lines[3])
	if len(fields) == 0 {
		return "<未知>"
	}
	return fields[0]
}
