// link_checker.java — Java версия

import okhttp3.*;
import okhttp3.HttpUrl;
import com.google.gson.Gson;
import com.google.gson.GsonBuilder;

import java.io.*;
import java.nio.file.*;
import java.util.*;
import java.util.concurrent.*;
import java.time.*;

public class link_checker {
    private static final OkHttpClient client;
    private static int maxRedirects;
    private static int timeout;

    static {
        timeout = 10;
        maxRedirects = 10;
        client = new OkHttpClient.Builder()
                .connectTimeout(timeout, TimeUnit.SECONDS)
                .readTimeout(timeout, TimeUnit.SECONDS)
                .followRedirects(false)
                .followSslRedirects(false)
                .build();
    }

    static class RedirectStep {
        String url;
        int statusCode;
        Map<String, String> headers;

        RedirectStep(String url, int statusCode, Map<String, String> headers) {
            this.url = url;
            this.statusCode = statusCode;
            this.headers = headers;
        }
    }

    static class Result {
        String url;
        String finalUrl;
        int statusCode;
        List<RedirectStep> redirects = new ArrayList<>();
        double time;
        long size;
        String error;
    }

    public static Result checkUrl(String urlStr) throws IOException {
        long start = System.currentTimeMillis();
        Result result = new Result();
        result.url = urlStr;

        String currentUrl = urlStr;
        int redirectCount = 0;

        try {
            while (redirectCount <= maxRedirects) {
                Request request = new Request.Builder()
                        .url(currentUrl)
                        .build();
                Response response = client.newCall(request).execute();
                int status = response.code();
                result.redirects.add(new RedirectStep(currentUrl, status, response.headers().toMultimap()));

                if (status >= 300 && status < 400) {
                    String location = response.header("Location");
                    if (location == null) break;
                    // Разрешаем относительный URL
                    HttpUrl base = HttpUrl.parse(currentUrl);
                    HttpUrl resolved = base.resolve(location);
                    if (resolved == null) break;
                    currentUrl = resolved.toString();
                    redirectCount++;
                    if (redirectCount > maxRedirects) {
                        result.error = "Слишком много редиректов (> " + maxRedirects + ")";
                        break;
                    }
                } else {
                    result.statusCode = status;
                    result.finalUrl = currentUrl;
                    result.size = response.body().bytes().length;
                    break;
                }
                response.close();
            }
            if (redirectCount > maxRedirects && result.error == null) {
                result.error = "Превышен лимит редиректов (" + maxRedirects + ")";
            }
        } catch (Exception e) {
            result.error = e.getMessage();
        }

        result.time = (System.currentTimeMillis() - start) / 1000.0;
        return result;
    }

    public static Map<String, Result> checkMultiple(List<String> urls, int workers) throws InterruptedException {
        Map<String, Result> results = new ConcurrentHashMap<>();
        ExecutorService executor = Executors.newFixedThreadPool(workers);
        List<Future<?>> futures = new ArrayList<>();

        for (String url : urls) {
            futures.add(executor.submit(() -> {
                try {
                    Result r = checkUrl(url);
                    results.put(url, r);
                } catch (IOException e) {
                    Result r = new Result();
                    r.url = url;
                    r.error = e.getMessage();
                    results.put(url, r);
                }
            }));
        }

        for (Future<?> f : futures) {
            f.get();
        }
        executor.shutdown();
        executor.awaitTermination(60, TimeUnit.SECONDS);
        return results;
    }

    public static void printResults(Map<String, Result> results) {
        for (Map.Entry<String, Result> entry : results.entrySet()) {
            String url = entry.getKey();
            Result r = entry.getValue();
            System.out.println("\n\u001B[36m🔍 Проверка: " + url + "\u001B[0m");
            if (r.error != null) {
                System.out.println("\u001B[31m❌ Ошибка: " + r.error + "\u001B[0m");
                continue;
            }
            String statusColor = "\u001B[32m";
            String statusText = r.statusCode + " OK";
            if (r.statusCode >= 300 && r.statusCode < 400) {
                statusColor = "\u001B[33m";
                statusText = r.statusCode + " Redirect";
            } else if (r.statusCode >= 400) {
                statusColor = "\u001B[31m";
                statusText = r.statusCode + " Error";
            }
            System.out.println(statusColor + "✅ " + r.finalUrl + "\u001B[0m");
            System.out.println("   Статус: " + statusColor + statusText + "\u001B[0m");
            System.out.printf("   Время: %.2f сек\n", r.time);
            System.out.println("   Размер: " + r.size + " байт");
            if (!r.redirects.isEmpty()) {
                System.out.println("   Редиректы: " + r.redirects.size());
                for (int i = 0; i < r.redirects.size(); i++) {
                    RedirectStep s = r.redirects.get(i);
                    System.out.printf("     %d. %s -> %d\n", i+1, s.url, s.statusCode);
                }
            }
        }
    }

    public static void saveJSON(Map<String, Result> results, String filename) throws IOException {
        Gson gson = new GsonBuilder().setPrettyPrinting().create();
        String json = gson.toJson(results);
        Files.write(Paths.get(filename), json.getBytes());
        System.out.println("\u001B[32m💾 Сохранено JSON: " + filename + "\u001B[0m");
    }

    public static void saveCSV(Map<String, Result> results, String filename) throws IOException {
        StringBuilder sb = new StringBuilder();
        sb.append("URL,Final URL,Status,Time (s),Size (bytes),Redirects,Error\n");
        for (Map.Entry<String, Result> entry : results.entrySet()) {
            Result r = entry.getValue();
            sb.append(entry.getKey()).append(",");
            sb.append(r.finalUrl != null ? r.finalUrl : "").append(",");
            sb.append(r.statusCode).append(",");
            sb.append(String.format("%.3f", r.time)).append(",");
            sb.append(r.size).append(",");
            sb.append(r.redirects.size()).append(",");
            sb.append(r.error != null ? r.error : "").append("\n");
        }
        Files.write(Paths.get(filename), sb.toString().getBytes());
        System.out.println("\u001B[32m💾 Сохранено CSV: " + filename + "\u001B[0m");
    }

    public static void main(String[] args) throws Exception {
        // Простой разбор аргументов (для демонстрации)
        List<String> urls = new ArrayList<>();
        String filePath = null;
        String outputJson = null;
        String outputCsv = null;
        int workers = 10;

        for (int i = 0; i < args.length; i++) {
            if (args[i].equals("--file") || args[i].equals("-f")) {
                filePath = args[++i];
            } else if (args[i].equals("--output-json")) {
                outputJson = args[++i];
            } else if (args[i].equals("--output-csv")) {
                outputCsv = args[++i];
            } else if (args[i].equals("--workers") || args[i].equals("-w")) {
                workers = Integer.parseInt(args[++i]);
            } else if (args[i].equals("--timeout")) {
                timeout = Integer.parseInt(args[++i]);
                // Обновить client
            } else if (args[i].equals("--max-redirects")) {
                maxRedirects = Integer.parseInt(args[++i]);
            } else if (!args[i].startsWith("-")) {
                urls.add(args[i]);
            }
        }

        if (filePath != null) {
            List<String> lines = Files.readAllLines(Paths.get(filePath));
            for (String line : lines) {
                line = line.trim();
                if (!line.isEmpty()) urls.add(line);
            }
        }

        if (urls.isEmpty()) {
            System.out.println("❌ Нет URL для проверки.");
            System.exit(1);
        }

        System.out.println("\u001B[36m🔗 Link Checker (Java)\u001B[0m");

        Map<String, Result> results = checkMultiple(urls, workers);
        printResults(results);

        if (outputJson != null) saveJSON(results, outputJson);
        if (outputCsv != null) saveCSV(results, outputCsv);
    }
}
