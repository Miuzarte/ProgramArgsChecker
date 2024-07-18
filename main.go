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
	chromeCopyName = "chromeCopy.exe"
	filtersName    = "filters.list"
)

var (
	user32          = syscall.MustLoadDLL("user32.dll")
	procMessageBoxW = user32.MustFindProc("MessageBoxW")
	xDir            string
	origPath        string
	copyPath        string
	filtersPath     string
	filters         []string
)

func init() {
	d, err := os.Executable()
	if err != nil {
		MessageBox(0, "panic！无法获取可执行文件路径", err.Error(), 0)
		panic(err)
	}
	xDir = filepath.Dir(d)
	origPath = filepath.Join(xDir, "chrome.exe")
	copyPath = filepath.Join(xDir, chromeCopyName)
	filtersPath = filepath.Join(xDir, filtersName)

	filtersF, err := os.Open(filtersPath)
	if err != nil {
		MessageBox(0, fmt.Sprintf("panic！找不到%s", filtersPath), err.Error(), 0)
		panic(err)
	}
	defer filtersF.Close()
	scanner := bufio.NewScanner(filtersF)
	for scanner.Scan() {
		filters = append(filters, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		MessageBox(0, "panic！过滤器获取失败", err.Error(), 0)
		panic(err)
	}
	if len(filters) == 0 {
		MessageBox(0, "panic！过滤器为空", fmt.Sprintf("请检查%s", filtersPath), 0)
	}

	fmt.Println("过滤列表:")
	for _, filter := range filters {
		fmt.Println(filter)
	}
}

func MessageBox(hwnd uintptr, title, caption string, flags uint) int {
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

func main() {
	// 除去前两个
	args := os.Args[2:]

	fmt.Println("调用方传参:")
	for _, a := range args {
		fmt.Println(a)
	}

	exists := false
	foundFilteredArgs := make([]string, 0)
	// 检查参数
loop:
	for _, arg := range args {
		for _, filter := range filters {
			if arg == filter {
				exists = true
				foundFilteredArgs = append(foundFilteredArgs, arg)
				break loop
			}
		}
	}

	if !exists {
		initAndRun(args)
	} else {
		ppid := syscall.Getppid()
		title := "发现黑名单参数:"
		for _, a := range foundFilteredArgs {
			title += " " + a
		}
		msg := fmt.Sprintf("调用方信息:\nPID: %d\n名称: %s", ppid, getProcessName(ppid))
		fmt.Printf("%s\n%s\n", title, msg)
		MessageBox(0, title, msg, 0)
	}
}

func initAndRun(args []string) {
	if err := checkChromeCopy(); err != nil {
		fmt.Println("初始化失败:\n", err)
		MessageBox(0, "初始化失败", err.Error(), 0)
		return
	}

	cmd := exec.Command(chromeCopyName, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Println("启动失败:\n", err)
		MessageBox(0, "启动失败", err.Error(), 0)
	}
}

func checkChromeCopy() error {
	copyFi, err := os.Stat(copyPath)
	if err != nil {
		fmt.Printf("%s不存在", chromeCopyName)
		return copyChromeWithModTime()
	}

	origFi, err := os.Stat(origPath)
	if err != nil {
		return fmt.Errorf("找不到原chrome.exe: %s", err.Error())
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
			fmt.Println("chrome.exe已变更")
			return copyChromeWithModTime()
		}
	}

	return nil
}

func copyChromeWithModTime() error {
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

func getProcessName(pid int) string {
	output, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid)).Output()
	if err != nil {
		return "未知进程名称"
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) < 4 {
		return "未知进程名称"
	}
	fields := strings.Fields(lines[3])
	if len(fields) == 0 {
		return "未知进程名称"
	}
	return fields[0]
}
