# ProgramArgsChecker
检查传入给程序的参数以实现阻止执行

硬编码了`chrome.exe`

需要放在目标程序的同一目录下

并通过注册表设置 Image File Execution Options (IFEO) 劫持

例如：

`HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options\chrome.exe`

(字符串值) `Debugger` - `C:\Users\MiuzarteVM\Desktop\chrome-win\ProgramArgsChecker.exe`
