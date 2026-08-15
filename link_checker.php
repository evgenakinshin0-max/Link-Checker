<?php
// link_checker.php — PHP версия

require_once 'vendor/autoload.php';

use GuzzleHttp\Client;
use GuzzleHttp\Exception\RequestException;
use GuzzleHttp\Psr7\Uri;
use GuzzleHttp\Psr7\UriResolver;

class LinkChecker
{
    private $client;
    private $maxRedirects;
    private $timeout;

    public function __construct($timeout = 10, $maxRedirects = 10, $headers = [])
    {
        $this->timeout = $timeout;
        $this->maxRedirects = $maxRedirects;
        $this->client = new Client([
            'timeout' => $timeout,
            'allow_redirects' => false,
            'headers' => $headers,
            'http_errors' => false,
        ]);
    }

    public function checkUrl($url)
    {
        $start = microtime(true);
        $result = [
            'url' => $url,
            'final_url' => null,
            'status_code' => null,
            'redirects' => [],
            'time' => 0,
            'size' => 0,
            'error' => null,
        ];

        $currentUrl = $url;
        $redirectCount = 0;

        try {
            while ($redirectCount <= $this->maxRedirects) {
                $response = $this->client->get($currentUrl);
                $status = $response->getStatusCode();
                $result['redirects'][] = [
                    'url' => $currentUrl,
                    'status_code' => $status,
                    'headers' => $response->getHeaders(),
                ];

                if ($status >= 300 && $status < 400) {
                    $location = $response->getHeaderLine('Location');
                    if (!$location) break;
                    // Разрешаем относительный URL
                    $base = new Uri($currentUrl);
                    $resolved = UriResolver::resolve($base, new Uri($location));
                    $currentUrl = (string) $resolved;
                    $redirectCount++;
                    if ($redirectCount > $this->maxRedirects) {
                        $result['error'] = "Слишком много редиректов (> {$this->maxRedirects})";
                        break;
                    }
                } else {
                    $result['status_code'] = $status;
                    $result['final_url'] = $currentUrl;
                    $result['size'] = strlen($response->getBody());
                    break;
                }
            }
            if ($redirectCount > $this->maxRedirects && !$result['error']) {
                $result['error'] = "Превышен лимит редиректов ({$this->maxRedirects})";
            }
        } catch (Exception $e) {
            $result['error'] = $e->getMessage();
        }

        $result['time'] = microtime(true) - $start;
        return $result;
    }

    public function checkMultiple($urls, $workers = 10)
    {
        $results = [];
        // Простая параллельная обработка через массивы (в PHP без многопоточности сложно)
        // Используем последовательную обработку для простоты
        foreach ($urls as $url) {
            $results[$url] = $this->checkUrl($url);
        }
        return $results;
    }
}

function printResults($results)
{
    foreach ($results as $url => $res) {
        echo "\n\033[36m🔍 Проверка: $url\033[0m\n";
        if ($res['error']) {
            echo "\033[31m❌ Ошибка: " . $res['error'] . "\033[0m\n";
            continue;
        }
        $status = $res['status_code'];
        $statusColor = ($status >= 200 && $status < 300) ? "\033[32m" :
                       (($status >= 300 && $status < 400) ? "\033[33m" : "\033[31m");
        $statusText = ($status >= 200 && $status < 300) ? "$status OK" :
                      (($status >= 300 && $status < 400) ? "$status Redirect" : "$status Error");
        echo $statusColor . "✅ " . $res['final_url'] . "\033[0m\n";
        echo "   Статус: " . $statusColor . $statusText . "\033[0m\n";
        printf("   Время: %.2f сек\n", $res['time']);
        echo "   Размер: " . $res['size'] . " байт\n";
        if (count($res['redirects']) > 0) {
            echo "   Редиректы: " . count($res['redirects']) . "\n";
            $i = 1;
            foreach ($res['redirects'] as $r) {
                echo "     $i. " . $r['url'] . " -> " . $r['status_code'] . "\n";
                $i++;
            }
        }
    }
}

function saveJSON($results, $filename)
{
    file_put_contents($filename, json_encode($results, JSON_PRETTY_PRINT | JSON_UNESCAPED_UNICODE));
    echo "\033[32m💾 Сохранено JSON: $filename\033[0m\n";
}

function saveCSV($results, $filename)
{
    $fp = fopen($filename, 'w');
    fputcsv($fp, ['URL', 'Final URL', 'Status', 'Time (s)', 'Size (bytes)', 'Redirects', 'Error']);
    foreach ($results as $url => $res) {
        fputcsv($fp, [
            $url,
            $res['final_url'] ?? '',
            $res['status_code'] ?? '',
            number_format($res['time'], 3),
            $res['size'] ?? 0,
            count($res['redirects']),
            $res['error'] ?? ''
        ]);
    }
    fclose($fp);
    echo "\033[32m💾 Сохранено CSV: $filename\033[0m\n";
}

function main($argv)
{
    $options = [
        'timeout' => 10,
        'max_redirects' => 10,
        'workers' => 10,
        'headers' => [],
    ];
    $urls = [];

    for ($i = 1; $i < count($argv); $i++) {
        if ($argv[$i] == '--file' || $argv[$i] == '-f') {
            $file = $argv[++$i];
            $lines = file($file, FILE_IGNORE_NEW_LINES | FILE_SKIP_EMPTY_LINES);
            foreach ($lines as $line) {
                $urls[] = trim($line);
            }
        } elseif ($argv[$i] == '--timeout' || $argv[$i] == '-t') {
            $options['timeout'] = (int)$argv[++$i];
        } elseif ($argv[$i] == '--max-redirects' || $argv[$i] == '-m') {
            $options['max_redirects'] = (int)$argv[++$i];
        } elseif ($argv[$i] == '--output-json' || $argv[$i] == '-o') {
            $options['output_json'] = $argv[++$i];
        } elseif ($argv[$i] == '--output-csv' || $argv[$i] == '-c') {
            $options['output_csv'] = $argv[++$i];
        } elseif ($argv[$i] == '--workers' || $argv[$i] == '-w') {
            $options['workers'] = (int)$argv[++$i];
        } elseif ($argv[$i] == '--header' || $argv[$i] == '-H') {
            $h = $argv[++$i];
            $parts = explode(':', $h, 2);
            if (count($parts) == 2) {
                $options['headers'][trim($parts[0])] = trim($parts[1]);
            }
        } elseif (!str_starts_with($argv[$i], '-')) {
            $urls[] = $argv[$i];
        }
    }

    if (empty($urls)) {
        echo "❌ Нет URL для проверки.\n";
        exit(1);
    }

    echo "\033[36m🔗 Link Checker (PHP)\033[0m\n";

    $checker = new LinkChecker($options['timeout'], $options['max_redirects'], $options['headers']);
    $results = $checker->checkMultiple($urls, $options['workers']);
    printResults($results);

    if (isset($options['output_json'])) saveJSON($results, $options['output_json']);
    if (isset($options['output_csv'])) saveCSV($results, $options['output_csv']);
}

$argc = $_SERVER['argc'] ?? 0;
$argv = $_SERVER['argv'] ?? [];
main($argv);
?>
