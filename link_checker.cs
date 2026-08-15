// link_checker.cs — C# версия

using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using System.Threading.Tasks;
using System.Threading;
using System.Diagnostics;

class RedirectStep
{
    public string Url { get; set; }
    public int StatusCode { get; set; }
    public Dictionary<string, string> Headers { get; set; }
}

class Result
{
    public string Url { get; set; }
    public string FinalUrl { get; set; }
    public int StatusCode { get; set; }
    public List<RedirectStep> Redirects { get; set; } = new List<RedirectStep>();
    public double Time { get; set; }
    public long Size { get; set; }
    public string Error { get; set; }
}

class LinkChecker
{
    private HttpClient client;
    private int maxRedirects;
    private int timeout;

    public LinkChecker(int timeoutSec, int maxRedirects, Dictionary<string, string> headers)
    {
        this.timeout = timeoutSec;
        this.maxRedirects = maxRedirects;
        var handler = new HttpClientHandler()
        {
            AllowAutoRedirect = false,
        };
        client = new HttpClient(handler);
        client.Timeout = TimeSpan.FromSeconds(timeoutSec);
        foreach (var h in headers)
            client.DefaultRequestHeaders.Add(h.Key, h.Value);
    }

    public async Task<Result> CheckUrl(string url)
    {
        var stopwatch = Stopwatch.StartNew();
        var result = new Result { Url = url };

        string currentUrl = url;
        int redirectCount = 0;

        try
        {
            while (redirectCount <= maxRedirects)
            {
                var response = await client.GetAsync(currentUrl);
                int status = (int)response.StatusCode;
                result.Redirects.Add(new RedirectStep
                {
                    Url = currentUrl,
                    StatusCode = status,
                    Headers = response.Headers.ToDictionary(h => h.Key, h => string.Join(",", h.Value))
                });

                if (status >= 300 && status < 400)
                {
                    var location = response.Headers.Location?.ToString();
                    if (string.IsNullOrEmpty(location))
                        break;
                    // Разрешаем относительный URL
                    var baseUri = new Uri(currentUrl);
                    var resolved = new Uri(baseUri, location);
                    currentUrl = resolved.ToString();
                    redirectCount++;
                    if (redirectCount > maxRedirects)
                    {
                        result.Error = $"Слишком много редиректов (> {maxRedirects})";
                        break;
                    }
                }
                else
                {
                    result.StatusCode = status;
                    result.FinalUrl = currentUrl;
                    var content = await response.Content.ReadAsByteArrayAsync();
                    result.Size = content.Length;
                    break;
                }
                response.Dispose();
            }
            if (redirectCount > maxRedirects && result.Error == null)
                result.Error = $"Превышен лимит редиректов ({maxRedirects})";
        }
        catch (Exception ex)
        {
            result.Error = ex.Message;
        }

        result.Time = stopwatch.Elapsed.TotalSeconds;
        return result;
    }

    public async Task<Dictionary<string, Result>> CheckMultiple(List<string> urls, int workers)
    {
        var results = new Dictionary<string, Result>();
        var sem = new SemaphoreSlim(workers);
        var tasks = urls.Select(async url =>
        {
            await sem.WaitAsync();
            try
            {
                var r = await CheckUrl(url);
                lock (results) results[url] = r;
            }
            finally { sem.Release(); }
        });
        await Task.WhenAll(tasks);
        return results;
    }
}

class Program
{
    static void PrintResults(Dictionary<string, Result> results)
    {
        foreach (var kv in results)
        {
            var url = kv.Key;
            var r = kv.Value;
            Console.WriteLine($"\n\u001B[36m🔍 Проверка: {url}\u001B[0m");
            if (!string.IsNullOrEmpty(r.Error))
            {
                Console.WriteLine($"\u001B[31m❌ Ошибка: {r.Error}\u001B[0m");
                continue;
            }
            string statusColor = "\u001B[32m";
            string statusText = $"{r.StatusCode} OK";
            if (r.StatusCode >= 300 && r.StatusCode < 400)
            {
                statusColor = "\u001B[33m";
                statusText = $"{r.StatusCode} Redirect";
            }
            else if (r.StatusCode >= 400)
            {
                statusColor = "\u001B[31m";
                statusText = $"{r.StatusCode} Error";
            }
            Console.WriteLine($"{statusColor}✅ {r.FinalUrl}\u001B[0m");
            Console.WriteLine($"   Статус: {statusColor}{statusText}\u001B[0m");
            Console.WriteLine($"   Время: {r.Time:F2} сек");
            Console.WriteLine($"   Размер: {r.Size} байт");
            if (r.Redirects.Count > 0)
            {
                Console.WriteLine($"   Редиректы: {r.Redirects.Count}");
                for (int i = 0; i < r.Redirects.Count; i++)
                    Console.WriteLine($"     {i+1}. {r.Redirects[i].Url} -> {r.Redirects[i].StatusCode}");
            }
        }
    }

    static void SaveJSON(Dictionary<string, Result> results, string filename)
    {
        var json = JsonSerializer.Serialize(results, new JsonSerializerOptions { WriteIndented = true });
        File.WriteAllText(filename, json);
        Console.WriteLine($"\u001B[32m💾 Сохранено JSON: {filename}\u001B[0m");
    }

    static void SaveCSV(Dictionary<string, Result> results, string filename)
    {
        var sb = new StringBuilder();
        sb.AppendLine("URL,Final URL,Status,Time (s),Size (bytes),Redirects,Error");
        foreach (var kv in results)
        {
            var r = kv.Value;
            sb.AppendLine($"{kv.Key},{r.FinalUrl ?? ""},{r.StatusCode},{r.Time:F3},{r.Size},{r.Redirects.Count},{r.Error ?? ""}");
        }
        File.WriteAllText(filename, sb.ToString());
        Console.WriteLine($"\u001B[32m💾 Сохранено CSV: {filename}\u001B[0m");
    }

    static async Task Main(string[] args)
    {
        var urls = new List<string>();
        string filePath = null;
        string outputJson = null;
        string outputCsv = null;
        int workers = 10;
        int timeout = 10;
        int maxRedirects = 10;
        var headers = new Dictionary<string, string>();

        for (int i = 0; i < args.Length; i++)
        {
            if (args[i] == "--file" || args[i] == "-f") filePath = args[++i];
            else if (args[i] == "--output-json") outputJson = args[++i];
            else if (args[i] == "--output-csv") outputCsv = args[++i];
            else if (args[i] == "--workers" || args[i] == "-w") workers = int.Parse(args[++i]);
            else if (args[i] == "--timeout") timeout = int.Parse(args[++i]);
            else if (args[i] == "--max-redirects") maxRedirects = int.Parse(args[++i]);
            else if (args[i] == "--header" || args[i] == "-H")
            {
                var h = args[++i];
                var parts = h.Split(':', 2);
                if (parts.Length == 2) headers[parts[0].Trim()] = parts[1].Trim();
            }
            else if (!args[i].StartsWith("-"))
                urls.Add(args[i]);
        }

        if (filePath != null)
        {
            var lines = File.ReadAllLines(filePath);
            foreach (var line in lines)
                if (!string.IsNullOrWhiteSpace(line)) urls.Add(line.Trim());
        }

        if (urls.Count == 0)
        {
            Console.WriteLine("❌ Нет URL для проверки.");
            return;
        }

        Console.WriteLine("\u001B[36m🔗 Link Checker (C#)\u001B[0m");

        var checker = new LinkChecker(timeout, maxRedirects, headers);
        var results = await checker.CheckMultiple(urls, workers);
        PrintResults(results);

        if (outputJson != null) SaveJSON(results, outputJson);
        if (outputCsv != null) SaveCSV(results, outputCsv);
    }
}
