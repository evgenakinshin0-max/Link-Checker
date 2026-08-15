// link_checker.js — JavaScript версия

const axios = require('axios');
const chalk = require('chalk');
const fs = require('fs');
const readline = require('readline');
const { program } = require('commander');

class LinkChecker {
    constructor(timeout = 10000, maxRedirects = 10, headers = {}) {
        this.timeout = timeout;
        this.maxRedirects = maxRedirects;
        this.headers = headers;
        this.client = axios.create({
            timeout: timeout,
            maxRedirects: 0, // отключаем автоматические редиректы
            headers: headers,
        });
    }

    async checkUrl(url) {
        const start = Date.now();
        const result = {
            url,
            finalUrl: null,
            statusCode: null,
            redirects: [],
            time: 0,
            size: 0,
            error: null,
        };

        let currentUrl = url;
        let redirectCount = 0;

        try {
            while (redirectCount <= this.maxRedirects) {
                const response = await this.client.get(currentUrl, {
                    validateStatus: () => true, // не выбрасывать ошибки для любых статусов
                });
                const status = response.status;
                result.redirects.push({
                    url: currentUrl,
                    statusCode: status,
                    headers: response.headers,
                });

                if (status >= 300 && status < 400) {
                    const location = response.headers.location;
                    if (!location) break;
                    // Обработка относительных редиректов
                    const resolved = new URL(location, currentUrl);
                    currentUrl = resolved.href;
                    redirectCount++;
                    if (redirectCount > this.maxRedirects) {
                        result.error = `Слишком много редиректов (> ${this.maxRedirects})`;
                        break;
                    }
                } else {
                    result.statusCode = status;
                    result.finalUrl = currentUrl;
                    result.size = response.data.length;
                    break;
                }
            }
            if (redirectCount > this.maxRedirects && !result.error) {
                result.error = `Превышен лимит редиректов (${this.maxRedirects})`;
            }
        } catch (err) {
            result.error = err.message;
        }

        result.time = (Date.now() - start) / 1000;
        return result;
    }

    async checkMultiple(urls, workers = 10) {
        const results = {};
        const concurrency = Math.min(workers, urls.length);
        const chunks = [];
        for (let i = 0; i < urls.length; i += concurrency) {
            chunks.push(urls.slice(i, i + concurrency));
        }
        for (const chunk of chunks) {
            const promises = chunk.map(async (url) => {
                results[url] = await this.checkUrl(url);
            });
            await Promise.all(promises);
        }
        return results;
    }
}

function printResults(results) {
    for (const [url, data] of Object.entries(results)) {
        console.log(`\n${chalk.cyan(`🔍 Проверка: ${url}`)}`);
        if (data.error) {
            console.log(chalk.red(`❌ Ошибка: ${data.error}`));
            continue;
        }
        const statusColor = data.statusCode >= 200 && data.statusCode < 300 ? chalk.green :
                           data.statusCode >= 300 && data.statusCode < 400 ? chalk.yellow : chalk.red;
        const statusText = data.statusCode >= 200 && data.statusCode < 300 ? `${data.statusCode} OK` :
                           data.statusCode >= 300 && data.statusCode < 400 ? `${data.statusCode} Redirect` :
                           `${data.statusCode} Error`;
        console.log(`${statusColor(`✅ ${data.finalUrl}`)}`);
        console.log(`   Статус: ${statusColor(statusText)}`);
        console.log(`   Время: ${data.time.toFixed(2)} сек`);
        console.log(`   Размер: ${data.size} байт`);
        if (data.redirects.length > 0) {
            console.log(`   Редиректы: ${data.redirects.length}`);
            data.redirects.forEach((r, i) => {
                console.log(`     ${i+1}. ${r.url} -> ${r.statusCode}`);
            });
        }
    }
}

function saveJSON(results, filename) {
    fs.writeFileSync(filename, JSON.stringify(results, null, 2));
    console.log(chalk.green(`💾 Сохранено JSON: ${filename}`));
}

function saveCSV(results, filename) {
    const rows = [['URL', 'Final URL', 'Status', 'Time (s)', 'Size (bytes)', 'Redirects', 'Error']];
    for (const [url, data] of Object.entries(results)) {
        rows.push([
            url,
            data.finalUrl || '',
            data.statusCode || '',
            data.time.toFixed(3),
            data.size || 0,
            data.redirects.length,
            data.error || '',
        ]);
    }
    const csv = rows.map(row => row.join(',')).join('\n');
    fs.writeFileSync(filename, csv);
    console.log(chalk.green(`💾 Сохранено CSV: ${filename}`));
}

async function main() {
    program
        .argument('[urls...]', 'Список URL')
        .option('-f, --file <file>', 'Файл со списком URL')
        .option('-t, --timeout <seconds>', 'Таймаут (сек)', parseInt, 10)
        .option('-m, --max-redirects <n>', 'Максимум редиректов', parseInt, 10)
        .option('-o, --output-json <file>', 'Сохранить в JSON')
        .option('-c, --output-csv <file>', 'Сохранить в CSV')
        .option('-w, --workers <n>', 'Количество потоков', parseInt, 10)
        .option('-H, --header <header>', 'Заголовки (key:value)', (val, arr) => { arr.push(val); return arr; }, [])
        .parse();

    const options = program.opts();
    const urlsArg = program.args;

    console.log(chalk.cyan('🔗 Link Checker (JavaScript)'));

    let urls = [];
    if (urlsArg.length) urls.push(...urlsArg);
    if (options.file) {
        const content = fs.readFileSync(options.file, 'utf-8');
        const lines = content.split('\n').map(l => l.trim()).filter(l => l);
        urls.push(...lines);
    }
    if (urls.length === 0) {
        console.log('❌ Нет URL для проверки.');
        process.exit(1);
    }

    const headers = {};
    options.header.forEach(h => {
        if (h.includes(':')) {
            const [key, value] = h.split(':', 2);
            headers[key.trim()] = value.trim();
        }
    });

    const checker = new LinkChecker(options.timeout * 1000, options.maxRedirects, headers);
    const results = await checker.checkMultiple(urls, options.workers);
    printResults(results);

    if (options.outputJson) saveJSON(results, options.outputJson);
    if (options.outputCsv) saveCSV(results, options.outputCsv);
}

main().catch(console.error);
