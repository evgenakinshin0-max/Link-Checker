

### 1. `link_checker.py` (Python)

```python
# link_checker.py — Python версия

import sys
import argparse
import json
import csv
import time
import requests
from concurrent.futures import ThreadPoolExecutor, as_completed
from urllib.parse import urljoin, urlparse
from colorama import init, Fore, Style

init(autoreset=True)

class LinkChecker:
    def __init__(self, timeout=10, max_redirects=10, headers=None):
        self.timeout = timeout
        self.max_redirects = max_redirects
        self.headers = headers or {}
        self.session = requests.Session()
        self.session.headers.update(self.headers)

    def check_url(self, url):
        """Проверяет один URL, отслеживая редиректы."""
        start_time = time.time()
        result = {
            'url': url,
            'final_url': None,
            'status_code': None,
            'redirects': [],
            'time': 0,
            'size': 0,
            'error': None
        }

        current_url = url
        redirect_count = 0

        try:
            while redirect_count <= self.max_redirects:
                response = self.session.get(current_url, timeout=self.timeout, allow_redirects=False, stream=True)
                status = response.status_code
                result['redirects'].append({
                    'url': current_url,
                    'status_code': status,
                    'headers': dict(response.headers)
                })

                if 300 <= status < 400:
                    location = response.headers.get('Location')
                    if not location:
                        break
                    # Обработка относительных редиректов
                    if not urlparse(location).netloc:
                        location = urljoin(current_url, location)
                    current_url = location
                    redirect_count += 1
                    if redirect_count > self.max_redirects:
                        result['error'] = f'Слишком много редиректов (> {self.max_redirects})'
                        break
                else:
                    result['status_code'] = status
                    result['final_url'] = current_url
                    result['size'] = len(response.content) if response.content else 0
                    break
            else:
                result['error'] = f'Превышен лимит редиректов ({self.max_redirects})'

        except requests.exceptions.RequestException as e:
            result['error'] = str(e)
        except Exception as e:
            result['error'] = f'Неизвестная ошибка: {e}'

        result['time'] = time.time() - start_time
        return result

    def check_multiple(self, urls, workers=10):
        """Проверяет несколько URL параллельно."""
        results = {}
        with ThreadPoolExecutor(max_workers=workers) as executor:
            future_to_url = {executor.submit(self.check_url, url): url for url in urls}
            for future in as_completed(future_to_url):
                url = future_to_url[future]
                try:
                    results[url] = future.result()
                except Exception as e:
                    results[url] = {'error': str(e)}
        return results

    def print_results(self, results):
        """Выводит результаты в терминал с цветами."""
        for url, data in results.items():
            print(f"\n{Fore.CYAN}🔍 Проверка: {url}{Style.RESET_ALL}")
            if data.get('error'):
                print(f"{Fore.RED}❌ Ошибка: {data['error']}{Style.RESET_ALL}")
                continue

            status = data.get('status_code')
            final_url = data.get('final_url', url)
            time_taken = data.get('time', 0)
            size = data.get('size', 0)
            redirects = data.get('redirects', [])

            if status and 200 <= status < 300:
                status_color = Fore.GREEN
                status_text = f"{status} OK"
            elif status and 300 <= status < 400:
                status_color = Fore.YELLOW
                status_text = f"{status} Redirect"
            else:
                status_color = Fore.RED
                status_text = f"{status} Error" if status else "Error"

            print(f"{status_color}✅ {final_url}{Style.RESET_ALL}")
            print(f"   Статус: {status_color}{status_text}{Style.RESET_ALL}")
            print(f"   Время: {time_taken:.2f} сек")
            print(f"   Размер: {size} байт")
            if redirects:
                print(f"   Редиректы: {len(redirects)}")
                for i, r in enumerate(redirects, 1):
                    print(f"     {i}. {r['url']} -> {r['status_code']}")

    def save_json(self, results, filename):
        with open(filename, 'w', encoding='utf-8') as f:
            json.dump(results, f, indent=2, ensure_ascii=False)
        print(f"{Fore.GREEN}💾 Сохранено JSON: {filename}{Style.RESET_ALL}")

    def save_csv(self, results, filename):
        import csv
        with open(filename, 'w', newline='', encoding='utf-8') as f:
            writer = csv.writer(f)
            writer.writerow(['URL', 'Final URL', 'Status', 'Time (s)', 'Size (bytes)', 'Redirects', 'Error'])
            for url, data in results.items():
                writer.writerow([
                    url,
                    data.get('final_url', ''),
                    data.get('status_code', ''),
                    f"{data.get('time', 0):.3f}",
                    data.get('size', 0),
                    len(data.get('redirects', [])),
                    data.get('error', '')
                ])
        print(f"{Fore.GREEN}💾 Сохранено CSV: {filename}{Style.RESET_ALL}")

def main():
    parser = argparse.ArgumentParser(description='Link Checker (Redirects)')
    parser.add_argument('urls', nargs='*', help='Список URL для проверки')
    parser.add_argument('--file', '-f', help='Файл со списком URL (по одному на строку)')
    parser.add_argument('--timeout', type=int, default=10, help='Таймаут (сек)')
    parser.add_argument('--max-redirects', type=int, default=10, help='Максимум редиректов')
    parser.add_argument('--output-json', help='Сохранить в JSON')
    parser.add_argument('--output-csv', help='Сохранить в CSV')
    parser.add_argument('--workers', type=int, default=10, help='Количество потоков')
    parser.add_argument('--header', '-H', action='append', help='Добавить заголовок (key: value)')
    args = parser.parse_args()

    print(f"{Fore.CYAN}🔗 Link Checker (Python){Style.RESET_ALL}")

    # Сбор URL
    urls = []
    if args.urls:
        urls.extend(args.urls)
    if args.file:
        with open(args.file, 'r', encoding='utf-8') as f:
            for line in f:
                line = line.strip()
                if line:
                    urls.append(line)
    if not urls:
        print("❌ Нет URL для проверки.")
        sys.exit(1)

    # Заголовки
    headers = {}
    if args.header:
        for h in args.header:
            if ':' in h:
                key, value = h.split(':', 1)
                headers[key.strip()] = value.strip()

    checker = LinkChecker(timeout=args.timeout, max_redirects=args.max_redirects, headers=headers)
    results = checker.check_multiple(urls, workers=args.workers)
    checker.print_results(results)

    if args.output_json:
        checker.save_json(results, args.output_json)
    if args.output_csv:
        checker.save_csv(results, args.output_csv)

if __name__ == "__main__":
    main()
