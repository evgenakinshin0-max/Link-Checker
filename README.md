# 🔗 Link Checker (Redirects) — проверяй ссылки и следи за редиректами

> «Каждая ссылка — это путь, а редирект — это поворот»

**Link Checker (Redirects)** — это набор консольных утилит для проверки HTTP-ссылок с отслеживанием цепочек редиректов.  
Программа показывает каждый шаг редиректа, финальный URL, статус-коды, время ответа и размер страницы. Идеально для аудита сайтов, проверки битых ссылок и мониторинга доступности.

## 🚀 Особенности
- 🔍 Отслеживание цепочки редиректов (до 10 шагов).
- 📊 Отображение статус-кодов, времени ответа и размера контента.
- 🎨 Цветная индикация состояния (зелёный — OK, жёлтый — редирект, красный — ошибка).
- 📋 Поддержка нескольких ссылок из аргументов или файла.
- ⚡ Асинхронная/конкурентная проверка (где реализовано).
- 📤 Экспорт результатов в JSON и CSV.
- 🖥️ Поддержка пользовательских заголовков и таймаута.
- 🧠 Обработка относительных редиректов и зацикливаний.

## 🛠️ Установка и запуск

Для каждого языка — минимальные зависимости.

| Язык       | Зависимости                          | Команда запуска                         |
|------------|--------------------------------------|-----------------------------------------|
| Python     | `requests`, `colorama`               | `python link_checker.py https://example.com` |
| Go         | стандартная библиотека               | `go run link_checker.go https://example.com` |
| JavaScript | Node.js, `axios`, `chalk`            | `node link_checker.js https://example.com` |
| Java       | `okhttp`, `gson`                     | `javac -cp .:okhttp.jar:gson.jar ... && java ...` |
| C#         | `System.Net.Http`, `Newtonsoft.Json` | `dotnet run https://example.com`        |
| Rust       | `reqwest`, `serde_json`, `colored`   | `cargo run -- https://example.com`      |
| Ruby       | `http`, `json`, `colorize`           | `ruby link_checker.rb https://example.com` |
| PHP        | `guzzlehttp/guzzle`                  | `php link_checker.php https://example.com` |

## 📖 Пример использования

```bash
$ python link_checker.py --urls https://example.com,https://google.com --output json
Вывод:

text
🔗 Link Checker (Python)
🔍 Проверка: https://example.com

✅ https://example.com
   Статус: 200 OK
   Время: 0.23 сек
   Размер: 1256 байт
   Редиректы: 0

✅ https://google.com
   Статус: 301 Moved Permanently -> https://www.google.com/
   Время: 0.45 сек
   Редиректы: 1
   Конечный URL: https://www.google.com/

💾 Сохранено: results.json
💾 Сохранено: results.csv
🤝 Вклад
Принимаются улучшения, новые языки, фичи.

📜 Лицензия
MIT — используйте свободно.

Автор: Ваш покорный слуга
