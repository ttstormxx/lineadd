package main

import (
	"archive/zip"
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var version = "1.0"

var banner = `
_ _                      _     _ 
| (_)_ __   ___  __ _  __| | __| |
| | | '_ \ / _ \/ _\` + "`" + `|/ _\` + "`" + `|/ _\` + "`" + `|
| | | | | |  __/ (_| | (_| | (_| | version: ` + version + `
|_|_|_| |_|\___|\__,_|\__,_|\__,_|
                https://github.com/ttstormxx/lineadd`

func showBanner() {
	fmt.Println(banner)
	fmt.Println()
}

// * 配置文件解析
type Config struct {
	BaseDir string `yaml:"baseDir"`
	Items   map[string]struct {
		Dicts []string `yaml:"dicts"`
		Path  string   `yaml:"path"`
		Alias []string `yaml:"alias"`
	} `yaml:",inline"`
}

func parseConfig(configfile string) (Config, error) {
	// 读取配置文件
	content, err := os.ReadFile(configfile)
	if err != nil {
		fmt.Println("读取配置文件出错：", err)
		return Config{}, err
	}

	// 解析YAML
	var config Config
	err = yaml.Unmarshal(content, &config)
	if err != nil {
		// panic(err)
		return Config{}, err
	}

	return config, nil
}

// *参数解析
var (
	add          string
	del          string
	optype       string   //类型 add或del
	category     string   //操作的类 web user pass
	count        bool     //统计模式
	read         string   //读取模式
	stat         bool     //状态模式
	target       []string //添加行的目的文件
	file         string   //存在新行的文件
	line         string   //cmd输入的原始新行
	lines        []string //cmd输入的新行
	rollback     bool     //回滚到上一次
	single       string   //single模式
	singletarget string   //single模式操作目标
	query        bool     //查询某行是否在字典中
	silent       bool     //安静模式
	bak          bool     //备份模式
	reconfig     bool     //重新初始化
	firstrun     bool     //首次运行
	write        bool     //依据配置文件想字典根目录写入配置的字典

	BaseDir         string   //字典根目录
	BaseDirFromUser string   //用户-base输入的根目录
	LogDir          string   //日志目录 log
	BakDir          string   //备份目录 log/bak
	LineConfigPath  string   //配置文件路径
	LineConfigFile  string   //配置文件路径
	newlines        []string //新行 未去重
	uniqlines       []string //新行 去重
	iswindows       bool
)

func ParseImplement() {
	// 挂件
}

// 判断是否存在必备参数 模式是否唯一
func ValidMode() {

	num := 0
	if len(add) > 0 {
		num++
	}
	if len(del) > 0 {
		num++
	}
	if query {
		num++
		if len(line) == 0 {
			fmt.Println("line: " + line)
			fmt.Println("query模式(-q)必须指定待查数据(-l)")
			os.Exit(1)
		}

	}
	if len(read) > 0 {
		num++
		if len(single) == 0 {
			fmt.Println("read模式(-r)必须指定字典(-s)")
			os.Exit(1)
		}
	}
	if stat {
		num++
	}
	if count {
		num++
	}
	if bak {
		num++
	}
	if rollback {
		num++
	}
	if num > 0 {
		if len(BaseDirFromUser) > 0 {
			fmt.Println("-base选项仅在-config时有效")
		}
		if write {
			fmt.Println("-write选项仅在-config时有效")
		}
	}
	if reconfig {
		num++
		if len(BaseDirFromUser) > 0 && write {
			fmt.Println("-base选项和-write选项只能选一种")
			os.Exit(1)
		}
	}
	// judge
	if firstrun {
		return
	} else {
		if num == 0 {
			fmt.Println("请选择处理模式: add del count read backup stat query config")
			flag.Usage()
			os.Exit(1)
		} else if num > 1 {
			fmt.Println("只能选择1种模式: add del count read backup stat query config")
			os.Exit(1)

		}
	}

}

// 判断输入的目标类是否有效
func ParamValid(config Config) {
	var param string
	var isvalid bool
	ModeParse()

	var configtypekeys []string
	for k := range config.Items {
		configtypekeys = append(configtypekeys, k)
	}
	if optype == "add" || optype == "del" || optype == "read" {
		if len(add) > 0 {
			param = add
		} else if len(del) > 0 {
			param = del
		} else if len(read) > 0 {
			param = read
		}
		for _, v := range configtypekeys {
			if param == v {
				isvalid = true
				break
			}
		}
		if !isvalid {
			fmt.Println("目标类别在配置文件内不存在")
			os.Exit(1)
		}
	}
	category = param
	target = config.Items[category].Dicts
	// 初始化 Alias 用于别名支持
	GetAlias(config)
	// 判断single操作字典是否存在类别中
	if len(single) > 0 {
		if !IsSingleValid() {
			fmt.Println("指定的字典不在该类别中")
			os.Exit(1)
		}
	}
}

// 获得操作目标类别 设置optype
func ModeParse() {
	if stat {
		optype = "stat"
	} else if len(read) > 0 {
		optype = "read"

	} else if count {
		optype = "count"

	} else if bak {
		optype = "bak"

	} else if len(add) > 0 {
		optype = "add"
	} else if len(del) > 0 {
		optype = "del"
	} else if query {
		optype = "query"
	} else if rollback {
		optype = "rollback"
	} else if reconfig {
		optype = "reconfig"
	} else if firstrun {
		optype = "init"

	}
}
func usage() {
	usagetips := `
  -a 	  add 加行模式
  -d      delete 减行模式
  -c 	  count 统计【全部】字典情况 行数 大小
  -r 	  read 读取指定字典
  -b	  backup 备份全部字典
  -t	  status 字典状态 配置文件状态
  -s      single 指定单一字典
  -f      file 含有待处理数据的文件
  -l      line 命令行输入的待处理行(逗号分隔)
  -q      query 查询某行是否在字典中, 返回字典名和行数
  -config 重新初始化(遍历字典根目录初始化配置文件)
  -base   字典根目录(用于在-config时设置BaseDir)
  -write  依据配置文件初始化字典根目录(-config时使用)
  -silent 安静模式 一个挂件`
	fmt.Println(usagetips)
}
func FlagParse(config Config) {
	flag.Usage = usage
	flag.StringVar(&add, "a", "", "添加模式, 指定字典")
	flag.StringVar(&del, "d", "", "删除模式, 指定字典")
	flag.StringVar(&read, "r", "", "删除模式, 指定字典")
	flag.StringVar(&single, "s", "", "指定操作单一字典")
	flag.StringVar(&file, "f", "", "存在待处理行的文本文件")
	flag.StringVar(&line, "l", "", "待处理数据，逗号分隔")
	flag.BoolVar(&silent, "silent", false, "安静模式: 一个挂件")
	flag.BoolVar(&stat, "t", false, "状态")
	flag.BoolVar(&count, "c", false, "统计字典")
	flag.BoolVar(&bak, "b", false, "备份字典")
	flag.BoolVar(&query, "q", false, "查询某行(单行数据)是否在字典中")
	flag.BoolVar(&reconfig, "config", false, "重新初始化(遍历字典根目录初始化配置文件)")
	flag.BoolVar(&write, "write", false, "依据配置文件初始化字典根目录(-config时使用)")
	flag.StringVar(&BaseDirFromUser, "base", "", "字典根目录(用于在-config时设置BaseDir)")
	flag.Parse()

	//有效性判断
	ValidMode()
	ParamValid(config)

	if len(line) > 0 {
		lines = strings.Split(line, ",")
	}

}

// ?特殊模式
// lineadd web new.txt

// 获取目标类category后存储其相关信息
type Aliastruct struct {
	index int
	name  string
	alias string
}

var Alias = make(map[string]Aliastruct)

func GetAlias(config Config) {
	for i, v := range target {
		as := Alias[v]
		as.index = i + 1
		as.name = v
		if i < len(config.Items[category].Alias) {

			as.alias = config.Items[category].Alias[i]
		}
		Alias[v] = as
	}
}
func IsSingleValid() bool {
	for k := range Alias {
		if strconv.Itoa(Alias[k].index) == single {
			// fmt.Printf("目标为: %s 命中index: %d\n", k, Alias[k].index)
			singletarget = k
			return true
		} else if Alias[k].alias == single {
			// fmt.Printf("目标为: %s 命中alias: %s\n", k, Alias[k].alias)
			singletarget = k
			return true

		} else if Alias[k].name == single {
			// fmt.Printf("目标为: %s 命中name: %s\n", k, Alias[k].name)
			singletarget = k
			return true

		}
	}
	return false
}

// stat implement
func StatDisplay(configfile string) {
	config, err := parseConfig(configfile)
	if err != nil {
		fmt.Println(err)
	} else {
		// 配置文件位置
		fmt.Printf("配置文件: %s\n", LineConfigFile)
		// 输出配置项
		fmt.Println("BaseDir:", config.BaseDir)
		fmt.Println()
		for k := range config.Items {
			fmt.Println(k + ": ")
			v := reflect.ValueOf(config.Items[k])
			for i := 0; i < v.NumField(); i++ {
				field := v.Type().Field(i)
				value := v.Field(i)
				fmt.Printf("  %s: %v\n", field.Name, value.Interface())
			}
			fmt.Println()
		}
	}
}

// * 目录 文件查询相关
func RealDir(dir string) string {
	readDir := filepath.Join(BaseDir, dir)
	// fmt.Println("readDir: " + readDir)
	return readDir
}

// * 备份引擎
func backupFile(srcFile string) error {
	// 拼接有效路径
	srcFile = RealDir(srcFile)
	destDir := BakDir
	// 打开源文件
	// fmt.Println("srcFile: " + srcFile)
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	} else {
		// fmt.Println("打开文件" + srcFile + "成功")
	}
	defer src.Close()

	// 创建目标目录
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// 创建目标文件
	t := time.Now().Format("【2006-01-02-15-04-05】")
	destFile := t + filepath.Base(srcFile)
	// destFile := "web2.txt"
	destFile = filepath.Join(destDir, filepath.Base(destFile))
	// fmt.Println("destFile: " + destFile)
	dest, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer dest.Close()
	// fmt.Println("创建文件" + destFile + "成功")

	// 复制源文件到目标文件
	if _, err := io.Copy(dest, src); err != nil {
		return err
	} else {
		// fmt.Println("复制文件成功")
	}

	return nil
}

// * 日志引擎
// 日志轮转
func backupLogIfExceedsSize() error {
	if !fileExists(BaseDir) || !fileExists(LogDir) {
		return nil
	}
	maxSize := int64(5 * 1024 * 1024) // 5MB
	// 获取文件信息
	filename := filepath.Join(LogDir, "log.txt")
	fi, err := os.Stat(filename)
	if err != nil {
		return err
	}

	// 判断文件大小是否超过最大值
	if fi.Size() <= maxSize {
		return nil
	}

	// 备份原文件
	err = backupFile(filepath.Join("log", filepath.Base(filename)))
	if err != nil {
		return err
	}
	// 删除指定的文件
	err = os.Remove(filename)
	if err != nil {
		// 处理删除文件时发生的错误
		panic(err)
	}

	// 创建新的日志文件
	err = createFileIfNotExist(filename)
	if err != nil {
		return err
	}

	return nil
}

// 日志信息
func loginfo(info string) error {

	// 获取当前时间并格式化
	t := time.Now().Format("2006-01-02 15:04:05")

	// 将信息追加到日志文件中
	if !silent && info != breakline {
		fmt.Printf("[%s] %s\n", t, info)
		// return nil
	}
	if len(LogDir) > 0 && fileExists(BaseDir) {

		// logdir := filepath.Join(BaseDir, "log")
		logFile := "log.txt"
		logFile = filepath.Join(LogDir, logFile)
		// 打开日志文件，如果不存在则创建
		// 创建目标目录
		if err := os.MkdirAll(LogDir, 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := fmt.Fprintf(f, "[%s] %s\n", t, info); err != nil {
			return err
		}
	}

	return nil
}

// 读取文件成行
func readFileIntoLines(filename string) []string {
	// 打开文件
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("打开文件出错：", err)
		return nil
	}
	defer file.Close()

	// 创建一个 slice 用于存储读取的每行数据
	var lines []string

	// 创建一个 Scanner 对象来逐行读取文件中的数据
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// 将读取的每行数据添加到 slice 中
		lines = append(lines, scanner.Text())
	}

	// 检查是否遇到了读取错误
	if err := scanner.Err(); err != nil {
		fmt.Println("读取文件出错：", err)
		return nil
	}

	// 输出读取的每行数据
	return lines
}

// 写入行至文件
func writeLinesToFile(lines []string, filename string) error {
	// 创建一个新文件或截断一个已有的文件
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// 创建一个 Writer 对象来写入数据到文件中
	writer := bufio.NewWriter(file)

	// 将每行数据写入文件中
	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}

	// 将缓存中的数据刷入文件中
	return writer.Flush()
}

// 统计信息 结果展示用数据

// uniqlines
func removeDuplicates(s []string) []string {
	m := make(map[string]bool)
	for _, v := range s {
		m[v] = true
	}

	var result []string
	for k := range m {
		result = append(result, k)
	}
	return result
}

// in逻辑
func contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}

// 返回是否存在 存在则返回index 否则返回null
func findIndex(arr []string, target string) *int {
	for i, v := range arr {
		if v == target {
			return &i
		}
	}
	return nil
}

// 输入数据处理
func InputManage() []string {
	// 读取待处理行
	var newlines []string
	var newlinesfromfile []string
	var newlinesfromcmd []string
	var newlinesfrompipe []string
	var newlinesfromuserinput []string
	newlinesfrompipe = readFromPipe()
	if newlinesfrompipe == nil && len(file) == 0 && len(lines) == 0 {
		// fmt.Println("请输入待处理数据: ")
		loginfo("请输入待处理数据: ")
		newlinesfromuserinput = ReadLInesFromUserInput()
	} else {

		if newlinesfrompipe != nil {
			// fmt.Println("获取管道符输入数据成功: " + strings.Join(newlinesfrompipe, ","))
			loginfo("获取管道符输入数据: " + strings.Join(newlinesfrompipe, ","))
		}
		if len(file) > 0 {
			loginfo("读取待处理行文件: " + file)
			newlinesfromfile = readFileIntoLines(file)

		}
		if len(lines) > 0 {
			loginfo("读取输入的待处理行: " + strings.Join(lines, ","))
			newlinesfromcmd = lines
		}
	}
	// 合并cmd和file内容
	newlines = append(newlinesfromfile, newlinesfromcmd...)
	newlines = append(newlines, newlinesfrompipe...)
	newlines = append(newlines, newlinesfromuserinput...)
	// 去重
	uniqlines = removeDuplicates(newlines)
	return uniqlines
}

// * 加行引擎
type addinfo struct {
	orilines    []string
	inputlines  []string
	resultlines []string
	addedlines  []string
	duplines    []string
}

// pipe支持
func readFromPipe() []string {
	// 检查标准输入是否来自管道符
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return nil
	}

	// 创建一个新的 Scanner 对象来读取标准输入
	scanner := bufio.NewScanner(os.Stdin)

	// 创建一个空的 slice
	lines := []string{}

	// 读取标准输入中的每一行，并将其添加到 slice 中
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
	}

	// 检查是否发生错误
	if err := scanner.Err(); err != nil {
		return nil
	}

	// 如果有数据，则返回 slice
	if len(lines) > 0 {
		return lines
	}

	// 如果没有数据，则返回 nil
	return nil
}

// 用户输入 stdin
func ReadLInesFromUserInput() []string {

	scanner := bufio.NewScanner(os.Stdin)
	lines := []string{}

	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			// 遇到两个回车符结束输入
			break
		}

		lines = append(lines, line)
	}

	return lines
}

func ADDENGINE(dir string, v string) {
	if len(singletarget) == 0 {

		loginfo("正在处理：" + v)
	}
	// cudir, _ := os.Getwd()
	// loginfo("当前目录: " + cudir)
	err := backupFile(filepath.Join(dir, v))
	if err != nil {
		fmt.Println(err)
	}
	loginfo("备份文件成功: " + v)

	// 读取目标文件
	orilines := readFileIntoLines(filepath.Join(BaseDir, dir, v))
	loginfo("待处理行数: " + strconv.Itoa(len(newlines)) + ": " + strings.Join(newlines, ","))
	//行处理
	resultlines := addLines(newlines, orilines)
	// 写入目标文件
	writeLinesToFile(resultlines, filepath.Join(BaseDir, dir, v))
	loginfo(v + " 文件写入完毕")
	// 状态展示
	// 写入日志信息
	err = loginfo("处理完毕")
	if err != nil {
		fmt.Println(err)
	}
	// fmt.Println(resultlines)
	// fmt.Println()
}

func addStatus(status addinfo) bool {
	loginfo("原有行数: " + strconv.Itoa(len(status.orilines)))
	loginfo("处理后行数: " + strconv.Itoa(len(status.resultlines)))
	loginfo("输入的新行: " + strconv.Itoa(len(status.inputlines)) + ": " + strings.Join(status.inputlines, " "))
	loginfo("添加的新行: " + strconv.Itoa(len(status.addedlines)) + ": " + strings.Join(status.addedlines, " "))
	loginfo("重复行: " + strconv.Itoa(len(status.duplines)) + ": " + strings.Join(status.duplines, " "))
	// loginfo("old lines count: " + strconv.Itoa(len(status.orilines)))
	// loginfo("lines after merge count: " + strconv.Itoa(len(status.resultlines)))
	// loginfo("input lines count: " + strconv.Itoa(len(status.inputlines)) + ": " + strings.Join(status.inputlines, " "))
	// loginfo("added new lines count: " + strconv.Itoa(len(status.addedlines)) + ": " + strings.Join(status.addedlines, " "))
	// loginfo("dup lines count: " + strconv.Itoa(len(status.duplines)) + ": " + strings.Join(status.duplines, " "))
	return true
}

func addLines(newlines []string, orilines []string) []string {
	loginfo("开始加行处理")
	var linestoadd []string //新行
	var duplines []string   //重复行
	var status addinfo
	status.orilines = make([]string, len(orilines))
	copy(status.orilines, orilines)
	for _, new := range newlines {
		if !contains(orilines, new) {
			linestoadd = append(linestoadd, new)
			orilines = append(orilines, new)
		} else {
			duplines = append(duplines, new)
		}
	}
	//for status display
	status.inputlines = newlines
	status.resultlines = orilines
	status.addedlines = linestoadd
	status.duplines = duplines
	addStatus(status)
	// loginfo("加行处理结束")
	return orilines

}

// * 减行引擎
type removeinfo struct {
	orilines      []string
	inputlines    []string
	resultlines   []string
	linestosub    []string
	linesnotexist []string
}

func DELENGINE(dir string, v string) {
	if len(singletarget) == 0 {

		loginfo("正在处理：" + v)
	}
	// cudir, _ := os.Getwd()
	// loginfo("当前目录: " + cudir)
	err := backupFile(filepath.Join(dir, v))
	if err != nil {
		fmt.Println(err)
	}
	loginfo("备份文件成功: " + v)

	// 读取目标文件
	orilines := readFileIntoLines(filepath.Join(BaseDir, dir, v))
	loginfo("待【删除】处理行数: " + strconv.Itoa(len(newlines)) + ": " + strings.Join(newlines, ","))
	//行处理
	resultlines := removeLines(newlines, orilines)
	// 写入目标文件
	writeLinesToFile(resultlines, filepath.Join(BaseDir, dir, v))
	loginfo(v + " 文件写入完毕")
	// 状态展示
	// 写入日志信息
	err = loginfo("【删除】处理完毕")
	if err != nil {
		fmt.Println(err)
	}
	// fmt.Println(resultlines)
	// fmt.Println()
}
func removeStatus(status removeinfo) bool {
	loginfo("原有行数: " + strconv.Itoa(len(status.orilines)))
	loginfo("处理后行数: " + strconv.Itoa(len(status.resultlines)))
	loginfo("输入的行: " + strconv.Itoa(len(status.inputlines)) + ": " + strings.Join(status.inputlines, " "))
	loginfo("删除的行: " + strconv.Itoa(len(status.linestosub)) + ": " + strings.Join(status.linestosub, " "))
	loginfo("不存在行: " + strconv.Itoa(len(status.linesnotexist)) + ": " + strings.Join(status.linesnotexist, " "))
	// loginfo("old lines count: " + strconv.Itoa(len(status.orilines)))
	// loginfo("lines after sub count: " + strconv.Itoa(len(status.resultlines)))
	// loginfo("input lines: " + strconv.Itoa(len(status.inputlines)) + ": " + strings.Join(status.inputlines, " "))
	// loginfo("lines subed count: " + strconv.Itoa(len(status.linestosub)) + ": " + strings.Join(status.linestosub, " "))
	// loginfo("lines not exist count: " + strconv.Itoa(len(status.linesnotexist)) + ": " + strings.Join(status.linesnotexist, " "))
	return true
}

// 删除元素
func deleteElement(arr []string, target string) *[]string {
	if len(arr) == 0 {
		return nil
	} else if len(arr) == 1 {
		if arr[0] == target {
			return &[]string{}
		}
	} else {

		for i, v := range arr {
			if v == target {
				tmp := append(arr[:i], arr[i+1:]...)
				return &tmp
			}
		}
	}
	return nil
}

func removeLines(newlines []string, orilines []string) []string {
	loginfo("开始【删行】处理")
	var linestosub []string
	var linesnotexist []string
	var status removeinfo //for status display
	v := make([]string, len(orilines))
	copy(v, orilines)
	status.orilines = v
	for i, v := range newlines {
		if findIndex(orilines, v) != nil {
			linestosub = append(linestosub, newlines[i])
			orilines = *deleteElement(orilines, v)
		} else {
			linesnotexist = append(linesnotexist, newlines[i])

		}
	}
	//状态展示
	status.inputlines = newlines
	status.resultlines = orilines
	status.linestosub = linestosub
	status.linesnotexist = linesnotexist
	removeStatus(status)
	// loginfo("【删行】处理结束")
	return orilines
}

// read模式
func READMODE(dir, filename string) {
	lines := readFileIntoLines(filepath.Join(BaseDir, dir, filename))
	for _, v := range lines {
		fmt.Println(v)
	}
}

// count模式
func COUNTMODE(config Config) {
	for k := range config.Items {
		fmt.Printf("%s: %d\n", k, len(config.Items[k].Dicts))
		for _, dict := range config.Items[k].Dicts {
			file := filepath.Join(BaseDir, config.Items[k].Path, dict)
			// 获取文件信息
			fileInfo, err := os.Stat(file)
			if err != nil {
				fmt.Println("无法获取文件信息：", err)
				return
			}

			// 获取文件大小
			fileSize := humanizeSize(fileInfo.Size())
			filelines := readFileIntoLines(file)
			fmt.Printf("  %s: line: %d size: %s\n", dict, len(filelines), fileSize)
			// fmt.Printf("    line: %d \n", len(filelines))
			// fmt.Printf("    size: %d B\n", fileSize)

		}
	}
}

// bak 模式
func BACKUPMODE() {
	// 指定要压缩的目录路径
	dirPath := BaseDir

	// 指定保存zip文件的路径
	t := time.Now().Format("2006-01-02-15-04-05")
	destFile := "bak-" + t + ".zip"
	zipPath := filepath.Join(BaseDir, "log", "bak", destFile)
	// 指定要排除的目录
	excludeDir := filepath.Join(BaseDir, "log")

	// 创建zip文件
	// 创建目标目录
	if err := os.MkdirAll(filepath.Dir(zipPath), 0755); err != nil {
		fmt.Println(err)
		return
	}
	zipFile, err := os.Create(zipPath)
	if err != nil {
		fmt.Println("无法创建zip文件: ", err)
		return
	}
	defer zipFile.Close()

	// 创建zip写入器
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// 遍历目录中的所有文件和子目录
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		// 如果遍历到的是目录，则跳过
		// 如果遍历到的是要排除的目录，则跳过
		if info.IsDir() {
			if strings.HasPrefix(path, excludeDir) {
				return filepath.SkipDir
			} else {
				return nil
			}
		}

		// 打开文件
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// 创建zip文件中的文件
		zipFile, err := zipWriter.Create(path[len(dirPath)+1:])
		if err != nil {
			return err
		}

		// 将文件内容写入zip文件中
		_, err = io.Copy(zipFile, file)
		if err != nil {
			return err
		}

		return nil
	})

	loginfo("备份完成：" + zipPath)
}

// query 模式
func QUERYMODE(config Config) {
	var datapresent bool
	for k := range config.Items {
		for _, dict := range config.Items[k].Dicts {
			file := filepath.Join(BaseDir, config.Items[k].Path, dict)
			lines := readFileIntoLines(file)
			if index := findIndex(lines, line); index != nil {
				fmt.Printf("数据 %s 在 %s 类的 %s 中的第 %d 行\n", line, k, dict, *index+1)
				datapresent = true
			}

		}
	}
	if !datapresent {
		fmt.Printf("数据 %s 不在任何字典中\n", line)
	}
}

// reconfig 模式
func SetConfig(baseDir string) (*Config, error) {
	t, _ := filepath.Abs(baseDir)
	baseDir = t
	// 初始化配置对象
	config := &Config{
		BaseDir: baseDir,
		Items: make(map[string]struct {
			Dicts []string `yaml:"dicts"`
			Path  string   `yaml:"path"`
			Alias []string `yaml:"alias"`
		}),
	}

	// 遍历根目录下的所有文件和文件夹
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// 如果是log目录，则跳过
		if info.IsDir() && info.Name() == "log" {
			return filepath.SkipDir
		}

		// 如果是文件夹，则设置为Items的键和Path的值
		if info.IsDir() {
			relPath, err := filepath.Rel(baseDir, path)
			if err != nil {
				return err
			}
			// 将相对路径中的反斜杠替换为正斜杠
			relPath = filepath.ToSlash(relPath)
			if relPath == "." {
				relPath = ""
			}
			// 在Path中保留相对目录，但Items的键中仅保留最后一级目录名
			relDir := filepath.Base(relPath)
			if relDir == "." {
				relDir = "base"
			}
			config.Items[relDir] = struct {
				// config.Items[relPath] = struct {
				Dicts []string `yaml:"dicts"`
				Path  string   `yaml:"path"`
				Alias []string `yaml:"alias"`
			}{
				Path: relPath,
			}
		} else {
			// 如果是文件，则设置为Dicts和Alias的值
			dir, file := filepath.Split(path)
			ext := filepath.Ext(file)
			fileName := strings.TrimSuffix(file, ext)
			relDir, err := filepath.Rel(baseDir, dir)
			if err != nil {
				return err
			}
			// 在Path中保留相对目录，但Items的键中仅保留最后一级目录名
			relDir = filepath.Base(relDir)
			if relDir == "." {
				relDir = "base"
			}
			item, ok := config.Items[relDir]
			if !ok {
				return fmt.Errorf("目录 %s 不存在", relDir)
			}
			item.Dicts = append(item.Dicts, file)
			item.Alias = append(item.Alias, fileName)
			config.Items[relDir] = item
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return config, nil
}
func writeConfig(config *Config) error {
	// 将Config对象序列化为YAML格式
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	// 将YAML数据写入文件
	err = os.WriteFile(LineConfigFile, data, 0644)
	if err != nil {
		return err
	}

	loginfo(fmt.Sprintf("成功将配置写入文件: %s", LineConfigFile))
	// fmt.Printf("成功将配置写入文件：%s\n", LineConfigFile)

	return nil
}
func RECONFIGMODE(config Config) {
	if write {
		loginfo("即将根据配置文件初始化字典根目录")

	} else if len(BaseDirFromUser) > 0 {
		// fmt.Printf("即将使用输入的根目录初始化配置文件: %s \n", BaseDirFromUser)
		loginfo(fmt.Sprintf("即将使用输入的根目录初始化配置文件: %s", BaseDirFromUser))
		BaseDir = BaseDirFromUser

	} else {

		loginfo(fmt.Sprintf("即将使用配置文件中的根目录初始化配置文件: %s", BaseDir))
	}
	// 如果读取到config参数，则提示用户进行二次确认
	loginfo("是否继续执行程序？(y/n) ")

	// 读取用户输入并进行处理
	var confirm string
	for {
		fmt.Scanln(&confirm)
		switch confirm {
		case "y":
			// loginfo("继续执行程序...")
			if write {
				loginfo("由配置文件初始化字典根目录")
				initBaseDir(config)
			} else {

				initConf(BaseDir)
			}
			return
		case "n":
			loginfo("取消执行程序...")
			return
		default:
			loginfo("无效的输入，请重新输入(y/n): ")
		}
	}
}

// OS 判断
func isWindows() bool {
	if runtime.GOOS == "windows" {
		return true
	} else {
		return false
	}
}

// 人性化显示文件大小
func humanizeSize(size int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	var i int
	var fSize float64 = float64(size)
	for i = 0; i < len(units)-1 && fSize >= 1024; i++ {
		fSize /= 1024
	}
	return fmt.Sprintf("%.2f %s", fSize, units[i])
}

// 文件是否存在
func fileExists(filename string) bool {
	// 判断文件是否存在
	_, err := os.Stat(filename)
	if err == nil {
		// 文件已存在，不需要创建
		return true
	} else if os.IsNotExist(err) {
		return false
	}
	return false
}

// 创建文件及其父文件夹
func createFileIfNotExist(filename string) error {
	// 判断文件是否存在
	_, err := os.Stat(filename)
	if err == nil {
		// 文件已存在，不需要创建
		return nil
	}
	// 文件不存在，尝试创建
	if os.IsNotExist(err) {
		// 获取文件所在目录
		dir := filepath.Dir(filename)
		// 创建目录和父目录
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		// 创建文件
		if _, err := os.Create(filename); err != nil {
			return err
		}
		// 创建成功
		return nil
	} else {
		// 其他错误，返回错误信息
		return err
	}
}

// 创建文件夹及其父文件夹
func createDirIfNotExist(dir string) error {
	// 判断文件夹是否存在
	_, err := os.Stat(dir)
	if err == nil {
		// 文件夹已存在，不需要创建
		return nil
	}
	// 文件夹不存在，尝试创建
	if os.IsNotExist(err) {
		// 创建目录和父目录
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
		// 创建成功
		return nil
	} else {
		// 其他错误，返回错误信息
		return err
	}
}

// init 遍历basedir初始化配置文件
func initConf(baseDir string) {
	newConfig, err := SetConfig(baseDir)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = writeConfig(newConfig)
	if err != nil {
		fmt.Println(err)
		return
	}
}

// init basedir 根据配置文件初始化字典根目录
func initBaseDir(config Config) {
	//创建basedir
	err := createDirIfNotExist(config.BaseDir)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	loginfo("创建根目录成功: " + BaseDir)
	for k := range config.Items {
		var t string
		if k != "base" {
			t = filepath.Join(config.BaseDir, config.Items[k].Path)
			err := createDirIfNotExist(t)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			loginfo("创建类目录成功: " + k)
		} else {
			t = config.BaseDir
		}
		for _, v := range config.Items[k].Dicts {
			dict := filepath.Join(t, v)
			err := createFileIfNotExist(dict)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			loginfo("创建字典文件成功: " + v)
		}
	}
	loginfo("初始化字典根目录成功")
}

func exampleConfFile() {
	// todo 初始化 未输入basedir时 写入示例配置文件
	fmt.Printf("写入示例配置到配置文件 %s", LineConfigFile)
}

func configInit() bool {
	if fileExists(LineConfigFile) {
		return false
	} else {
		firstrun = true
		// fmt.Println("首次执行，初始化配置文件")
		loginfo("首次执行，初始化配置文件")
		err := createFileIfNotExist(LineConfigFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		// fmt.Print("请输入字典根目录BaseDir: ")
		loginfo("请输入字典根目录BaseDir: ")
		var baseDir string
		fmt.Scanln(&baseDir)
		if baseDir == "" {
			//todo 输入baseDir为空时仅生成示例配置文件 提示后续自行填写
			// exampleConfFile()
		}
		t, err := filepath.Abs(baseDir)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		BaseDir = t
		initConf(BaseDir)
		// fmt.Printf("配置文件位置: %s\n", LineConfigFile)
		// fmt.Printf("字典根目录配置: %s\n", BaseDir)
		loginfo(fmt.Sprintf("配置文件位置: %s", LineConfigFile))
		loginfo(fmt.Sprintf("字典根目录配置: %s", BaseDir))
	}
	return true
}

var breakline = "======================================="

func main() {
	stime := time.Now()
	iswindows = isWindows()
	showBanner()
	// 获取用户名
	currentUser, err := user.Current()
	if err != nil {
		// fmt.Println("获取当前用户信息失败:", err)
		loginfo(fmt.Sprintf("获取当前用户信息失败: %s", err))
		return
	}
	parts := strings.Split(currentUser.Username, "\\")
	username := parts[len(parts)-1]
	// 配置文件
	configfile := "lineconfig.yaml"
	if iswindows {
		LineConfigPath = fmt.Sprintf("C:\\Users\\%s\\AppData\\Roaming\\lineadd", username)
	} else {
		LineConfigPath = fmt.Sprintf("/home/%s/.config/lineadd", username)

	}
	LineConfigFile = filepath.Join(LineConfigPath, configfile)
	// 初始化判断 是否存在配置文件 不存在则自动创建 并提示用户输入basedir
	isinit := configInit()
	if isinit {
		// fmt.Println("初始化结束")
		loginfo("初始化结束")
		return
	}
	// ubuntu  /home/user/.config/lineadd/LineConfigPath.yaml
	// windows "C:\\Users\\{username}\\AppData\\Roaming\\lineadd\\LineConfigFile.yaml"
	//todo 读取配置文件时确定指定的字典文件是否真实存在
	//todo 若不存在则自动创建
	// 初始化时自动读取配置的basedir下的目录和字典来初始化config文件
	config, err := parseConfig(LineConfigFile)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	BaseDir = config.BaseDir
	LogDir = filepath.Join(BaseDir, "log")
	BakDir = filepath.Join(LogDir, "bak")
	err = backupLogIfExceedsSize()
	if err != nil {
		loginfo(fmt.Sprint(err))
	}
	FlagParse(config)
	loginfo(breakline)
	// if silent {
	// 	fmt.Println("安静模式")
	// }
	// debug信息
	// fmt.Println("BaseDir: " + BaseDir)
	// fmt.Println("optype: " + optype)
	// fmt.Println("add: " + add)
	// fmt.Println("del: " + del)
	// fmt.Println("count: " + strconv.FormatBool(count))
	// fmt.Println("read: " + read)
	// fmt.Println("bak: " + strconv.FormatBool(bak))
	// fmt.Println("stat: " + strconv.FormatBool(stat))
	// fmt.Println("category: " + category)
	// fmt.Println("target: " + strings.Join(target, ","))
	// fmt.Println("file: " + file)
	// fmt.Println("line: " + line)
	// fmt.Println("lines: " + strings.Join(lines, ","))
	// fmt.Println("single: " + single)
	// fmt.Println("singletarget: " + singletarget)
	// fmt.Print("silent: " + strconv.FormatBool(silent))
	// fmt.Println()
	// 字典处理
	dir := config.Items[category].Path
	// dir = filepath.Join(".", dir)
	if optype == "reconfig" {
		RECONFIGMODE(config)
	} else if optype == "stat" {
		StatDisplay(LineConfigFile)
	} else if optype == "read" {
		READMODE(dir, singletarget)
	} else if optype == "count" {
		COUNTMODE(config)
	} else if optype == "bak" {
		BACKUPMODE()
	} else if optype == "query" {
		QUERYMODE(config)
	} else if optype == "rollback" || optype == "init" {
		// 挂件
	} else { //加减行模式
		//待处理数据输入
		loginfo("mode: " + optype)
		newlines = InputManage()
		if optype == "add" {
			// loginfo("mode: " + optype)
			loginfo("添加处理中")
			// 备份目标文件
			if len(singletarget) > 0 {
				loginfo("单文件处理: " + singletarget)
				v := singletarget
				ADDENGINE(dir, v)
			} else {

				for _, v := range config.Items[category].Dicts {
					loginfo("")
					ADDENGINE(dir, v)
				}
			}
		} else if optype == "del" {
			// loginfo("mode: " + optype)
			loginfo("删除处理中")
			// 备份目标文件
			if len(singletarget) > 0 {
				loginfo("单文件处理: " + singletarget)
				v := singletarget
				DELENGINE(dir, v)
			} else {

				for _, v := range config.Items[category].Dicts {
					loginfo("")
					DELENGINE(dir, v)
				}
			}
		}
	}
	loginfo(breakline)
	etime := time.Now()
	rtime := etime.Sub(stime)
	// fmt.Printf("程序运行时间: %s\n", rtime)
	if optype == "add" || optype == "del" || optype == "bak" {

		loginfo(fmt.Sprintf("程序运行时间: %s\n", rtime))
	}
}
