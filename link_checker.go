// link_checker.go — Go версия

package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type RedirectStep struct {
	URL        string            `json:"url"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
}

type Result struct {
	URL        string           `json:"url"`
	FinalURL   string           `json:"final_url"`
	StatusCode int              `json:"status_code"`
	Redirects  []RedirectStep   `json:"redirects"`
	Time       float64          `json:"time"`
	Size       int64            `json:"size"`
	Error      string           `json:"error,omitempty"`
}

type LinkChecker struct {
	Timeout      time.Duration
	MaxRedirects int
	Headers      map[string]string
	Client       *http.Client
}

func NewLinkChecker(timeout int, maxRedirects int, headers map[string]string) *LinkChecker {
	return &LinkChecker{
		Timeout:      time.Duration(timeout) * time.Second,
		MaxRedirects: maxRedirects,
		Headers:      headers,
		Client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("слишком много редиректов")
				}
				return nil
			},
		},
	}
}

func (lc *LinkChecker) checkURL(urlStr string) Result {
	start := time.Now()
	result := Result{URL: urlStr, Redirects: []RedirectStep{}}

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	for k, v := range lc.Headers {
		req.Header.Set(k, v)
	}

	resp, err := lc.Client.Do(req)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	// Собираем редиректы из истории
	history := []RedirectStep{}
	currentURL := urlStr
	for _, r := range resp.Request.Response {
		// Неполно, но мы можем отследить вручную
	}
	// Простейший вариант: используем resp.Request.URL как конечный
	finalURL := resp.Request.URL.String()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// Попытка получить Location
		loc := resp.Header.Get("Location")
		if loc != "" {
			// Добавляем шаг редиректа
			history = append(history, RedirectStep{
				URL:        currentURL,
				StatusCode: resp.StatusCode,
				Headers:    map[string]string{"Location": loc},
			})
			// Разрешаем относительный URL
			if !strings.HasPrefix(loc, "http") {
				base, _ := url.Parse(currentURL)
				rel, _ := url.Parse(loc)
				loc = base.ResolveReference(rel).String()
			}
			// Рекурсивно проверяем следующий URL
			nextResult := lc.checkURL(loc)
			result.FinalURL = nextResult.FinalURL
			result.StatusCode = nextResult.StatusCode
			result.Redirects = append(history, nextResult.Redirects...)
			result.Size = nextResult.Size
			result.Time = time.Since(start).Seconds()
			return result
		}
	}

	// Нет редиректа или финальный ответ
	result.FinalURL = finalURL
	result.StatusCode = resp.StatusCode
	body, _ := io.ReadAll(resp.Body)
	result.Size = int64(len(body))
	result.Time = time.Since(start).Seconds()
	return result
}

func (lc *LinkChecker) checkMultiple(urls []string, workers int) map[string]Result {
	results := make(map[string]Result)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := lc.checkURL(url)
			mu.Lock()
			results[url] = res
			mu.Unlock()
		}(u)
	}
	wg.Wait()
	return results
}

func printResults(results map[string]Result) {
	for url, res := range results {
		fmt.Printf("\n\x1b[36m🔍 Проверка: %s\x1b[0m\n", url)
		if res.Error != "" {
			fmt.Printf("\x1b[31m❌ Ошибка: %s\x1b[0m\n", res.Error)
			continue
		}
		statusColor := "\x1b[32m"
		statusText := fmt.Sprintf("%d OK", res.StatusCode)
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			statusColor = "\x1b[33m"
			statusText = fmt.Sprintf("%d Redirect", res.StatusCode)
		} else if res.StatusCode >= 400 {
			statusColor = "\x1b[31m"
			statusText = fmt.Sprintf("%d Error", res.StatusCode)
		}
		fmt.Printf("%s✅ %s\x1b[0m\n", statusColor, res.FinalURL)
		fmt.Printf("   Статус: %s%s\x1b[0m\n", statusColor, statusText)
		fmt.Printf("   Время: %.2f сек\n", res.Time)
		fmt.Printf("   Размер: %d байт\n", res.Size)
		if len(res.Redirects) > 0 {
			fmt.Printf("   Редиректы: %d\n", len(res.Redirects))
			for i, r := range res.Redirects {
				fmt.Printf("     %d. %s -> %d\n", i+1, r.URL, r.StatusCode)
			}
		}
	}
}

func saveJSON(results map[string]Result, filename string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func saveCSV(results map[string]Result, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	writer.Write([]string{"URL", "Final URL", "Status", "Time (s)", "Size (bytes)", "Redirects", "Error"})
	for u, r := range results {
		writer.Write([]string{
			u,
			r.FinalURL,
			fmt.Sprintf("%d", r.StatusCode),
			fmt.Sprintf("%.3f", r.Time),
			fmt.Sprintf("%d", r.Size),
			fmt.Sprintf("%d", len(r.Redirects)),
			r.Error,
		})
	}
	return nil
}

func main() {
	var urlsStr string
	var filePath string
	var timeout int
	var maxRedirects int
	var outputJSON string
	var outputCSV string
	var workers int
	var headersStr string

	flag.StringVar(&urlsStr, "urls", "", "Список URL через запятую")
	flag.StringVar(&filePath, "file", "", "Файл со списком URL")
	flag.IntVar(&timeout, "timeout", 10, "Таймаут (сек)")
	flag.IntVar(&maxRedirects, "max-redirects", 10, "Максимум редиректов")
	flag.StringVar(&outputJSON, "output-json", "", "Сохранить в JSON")
	flag.StringVar(&outputCSV, "output-csv", "", "Сохранить в CSV")
	flag.IntVar(&workers, "workers", 10, "Количество потоков")
	flag.StringVar(&headersStr, "header", "", "Заголовки (key1:value1,key2:value2)")
	flag.Parse()

	fmt.Println("\x1b[36m🔗 Link Checker (Go)\x1b[0m")

	var urls []string
	if urlsStr != "" {
		urls = append(urls, strings.Split(urlsStr, ",")...)
	}
	if filePath != "" {
		file, err := os.Open(filePath)
		if err != nil {
			fmt.Printf("❌ Ошибка открытия файла: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				urls = append(urls, line)
			}
		}
	}
	if len(urls) == 0 {
		fmt.Println("❌ Нет URL для проверки.")
		os.Exit(1)
	}

	headers := map[string]string{}
	if headersStr != "" {
		for _, h := range strings.Split(headersStr, ",") {
			if parts := strings.SplitN(h, ":", 2); len(parts) == 2 {
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	checker := NewLinkChecker(timeout, maxRedirects, headers)
	results := checker.checkMultiple(urls, workers)
	printResults(results)

	if outputJSON != "" {
		if err := saveJSON(results, outputJSON); err == nil {
			fmt.Printf("\x1b[32m💾 Сохранено JSON: %s\x1b[0m\n", outputJSON)
		}
	}
	if outputCSV != "" {
		if err := saveCSV(results, outputCSV); err == nil {
			fmt.Printf("\x1b[32m💾 Сохранено CSV: %s\x1b[0m\n", outputCSV)
		}
	}
}
