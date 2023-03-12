package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed citys.json
var cityJosnByte []byte
var keyword string

type Info struct {
	Name    string `json:"name"`
	Tel     string `json:"tel"`
	Address string `json:"address"`
	City    string `json:"city"`
}

var wg sync.WaitGroup
var Infos map[string][]Info

func main() {
	// 输出当前运行目录
	dir, _ := os.Getwd()
	log.Println(dir)

	fmt.Println("请输入关键字：")
	fmt.Scanf("%s\n", &keyword)
	if len(keyword) == 0 {
		log.Println("关键字不能为空")
		return
	}

	// 请输入要开始的页数
	var startPage int
	fmt.Println("请输入开始页数：")
	fmt.Scanf("%d\n", &startPage)
	if startPage == 0 {
		log.Println("开始页数不能为空")
		return
	}

	// 请输入要结束的页数
	var endPage int
	fmt.Println("请输入结束页数：")
	fmt.Scanf("%d\n", &endPage)
	if endPage == 0 {
		log.Println("结束页数不能为空")
		return
	}

	maxTasks := 5
	taskCh := make(chan int, maxTasks)
	// 控制协程数量
	// 考场总数759
	for i := startPage; i <= endPage; i++ {
		time.Sleep(3 * time.Second)
		wg.Add(1)
		taskCh <- 1
		go OpenBrowser(fmt.Sprintf("https://www.chahaoba.com/search_es?input=%s&page=%d", keyword, i), taskCh)
	}

	close(taskCh)

	wg.Wait()
	i := Infos
	fmt.Println(i)

	//	写入文件
	SaveData()

	log.Println("爬取完成")
}

func OpenBrowser(url string, taskCh <-chan int) {
	defer wg.Done()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
	)

	actx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(actx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// 设置超时时间
	ctx, _ = context.WithTimeout(ctx, 60*time.Second)

	var res []*cdp.Node
	err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.Nodes(`#es_res_box a[href]`, &res, chromedp.ByQueryAll))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// 超时重载
			log.Println("等待超时，开始重载页面")
			// 新建一个context
			ctx, cancel = chromedp.NewContext(actx, chromedp.WithLogf(log.Printf))
			defer cancel()
			// 设置超时时间
			ctx, _ = context.WithTimeout(ctx, 60*time.Second)
			err = chromedp.Run(ctx,
				chromedp.Navigate(url),
				chromedp.Nodes(`#es_res_box a[href]`, &res, chromedp.ByQueryAll),
			)
			if err != nil {
				log.Println("重载后依旧超时，放弃")
				log.Println(err)
			}
		} else {
			log.Println(err)
		}
	}

	for i := 0; i < len(res); i++ {
		// 打开a连接
		u := res[i].AttributeValue("href")
		log.Println(u)
		err = chromedp.Run(ctx, chromedp.Navigate(u))
		if err != nil {
			log.Println(fmt.Sprintf("打开%s失败，错误信息：", u) + err.Error())
			continue
		}

		// 获取第一个tbody 标签
		var content string
		err = chromedp.Run(ctx, chromedp.Text(`tbody:first-of-type`, &content, chromedp.ByQuery))
		if err != nil {
			log.Println(fmt.Sprintf("获取%s的tbody标签失败，错误信息：", u) + err.Error())
			continue
		}

		// 正则匹配电话，城市，名称，地址 等信息
		var info Info
		// 查找https://www.chahaoba.com/15870768653 的最后一个斜杆
		index := strings.LastIndex(u, "/")
		info.Tel = u[index+1:]

		// 匹配城市
		cityRe := regexp.MustCompile(`(?m)🏠️城市\s+([\p{Han}]+)`)
		cityMatch := cityRe.FindStringSubmatch(content)
		if len(cityMatch) > 1 {
			info.City = cityMatch[1]
		}

		// 匹配名称
		nameRe := regexp.MustCompile(`(?m)📝名称\s+([\p{Han}\w\d\s]+)`)
		nameMatch := nameRe.FindStringSubmatch(content)
		if len(nameMatch) > 1 {
			info.Name = nameMatch[1]
		}

		// 匹配地址
		addressRe := regexp.MustCompile(`(?m)📍地址\s+([\p{Han}\w\d\s]+)`)
		addressMatch := addressRe.FindStringSubmatch(content)
		if len(addressMatch) > 1 {
			info.Address = addressMatch[1]
		}

		if info.Tel != "" {
			var rw sync.Mutex
			rw.Lock()
			if Infos == nil {
				Infos = make(map[string][]Info)
			}
			_, ok := Infos[info.Tel]
			if !ok {
				Infos[info.Tel] = append(Infos[info.Tel], info)
			}
			rw.Unlock()
		}
	}
	log.Println(fmt.Sprintf("协程结束,已收到到%d个%s联系方式", len(Infos), keyword))
	<-taskCh
}

// SaveData 按城市保存数据
func SaveData() {
	// 转成map
	type Citys struct {
		Name string `json:"name"`
		City []struct {
			Name   string `json:"name"`
			County []struct {
				Name string `json:"name"`
			}
		}
	}
	var citys []Citys
	err := json.Unmarshal(cityJosnByte, &citys)
	if err != nil {
		log.Println("转换城市数据失败：" + err.Error())
		return
	}

	// 先匹配城市
	for _, v := range Infos {
		for _, kaoChang := range v {
			for _, city := range citys {
				for _, city1 := range city.City {
					if strings.Contains(kaoChang.Address, city1.Name) {
						// 保存数据
						WriteFile(city.Name+"_"+city1.Name, kaoChang)
						delete(Infos, kaoChang.Tel)
					} else if strings.Contains(kaoChang.Name, city1.Name) {
						// 保存数据
						WriteFile(city.Name+"_"+city1.Name, kaoChang)
						delete(Infos, kaoChang.Tel)
					}
				}
			}
		}
	}

	// 再保存已经有城市的数据
	for _, v := range Infos {
		for _, kaoChang := range v {
			if kaoChang.City != "" {
				// 保存数据
				WriteFile(kaoChang.City, kaoChang)
				delete(Infos, kaoChang.Tel)
			}
		}
	}

	// 最后匹配县
	for _, v := range Infos {
		for _, kaoChang := range v {
			for _, city := range citys {
				for _, city1 := range city.City {
					for _, county := range city1.County {
						if strings.Contains(kaoChang.Address, county.Name) {
							// 保存数据
							WriteFile(city.Name+"_"+county.Name, kaoChang)
							delete(Infos, kaoChang.Tel)
						} else if strings.Contains(kaoChang.Name, county.Name) {
							// 保存数据
							WriteFile(city.Name+"_"+county.Name, kaoChang)
							delete(Infos, kaoChang.Tel)
						}
					}
				}
			}
		}
	}

}

func WriteFile(name string, kaoChang Info) {
	fileName := fmt.Sprintf("%s/%s.txt", keyword, name)
	data, err := json.Marshal(kaoChang)
	if err != nil {
		log.Println("转换数据失败：" + err.Error())
		return
	}
	// 判断目录是否存在
	if _, err := os.Stat(keyword); os.IsNotExist(err) {
		// 创建目录
		err := os.Mkdir(keyword, os.ModePerm)
		if err != nil {
			log.Println("创建目录失败：" + err.Error())
			return
		}
	}
	// 打开文件 不存在则创建
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("Failed to open file:", err)
		return
	}
	defer file.Close()

	// 写入内容
	if _, err := file.WriteString(string(data) + "\n"); err != nil {
		fmt.Println("Failed to write to file:", err)
		return
	}
}
