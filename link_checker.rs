// link_checker.rs — Rust версия

use reqwest::Client;
use serde_json::json;
use std::collections::HashMap;
use std::fs::File;
use std::io::{self, BufRead, Write};
use std::time::Instant;
use url::Url;
use colored::*;
use clap::{App, Arg};

#[derive(Debug, Clone)]
struct RedirectStep {
    url: String,
    status_code: u16,
    headers: HashMap<String, String>,
}

#[derive(Debug, Clone)]
struct Result {
    url: String,
    final_url: Option<String>,
    status_code: Option<u16>,
    redirects: Vec<RedirectStep>,
    time: f64,
    size: u64,
    error: Option<String>,
}

async fn check_url(client: &Client, url_str: &str, max_redirects: usize) -> Result {
    let start = Instant::now();
    let mut result = Result {
        url: url_str.to_string(),
        final_url: None,
        status_code: None,
        redirects: Vec::new(),
        time: 0.0,
        size: 0,
        error: None,
    };

    let mut current_url = url_str.to_string();
    let mut redirect_count = 0;

    while redirect_count <= max_redirects {
        match client.get(&current_url).send().await {
            Ok(resp) => {
                let status = resp.status().as_u16();
                let headers = resp.headers().iter()
                    .map(|(k, v)| (k.to_string(), v.to_str().unwrap_or("").to_string()))
                    .collect();
                result.redirects.push(RedirectStep {
                    url: current_url.clone(),
                    status_code: status,
                    headers,
                });

                if (300..400).contains(&status) {
                    if let Some(location) = resp.headers().get("location") {
                        if let Ok(loc_str) = location.to_str() {
                            if let Ok(parsed) = Url::parse(&current_url).and_then(|base| base.join(loc_str)) {
                                current_url = parsed.to_string();
                                redirect_count += 1;
                                if redirect_count > max_redirects {
                                    result.error = Some(format!("Слишком много редиректов (> {})", max_redirects));
                                    break;
                                }
                                continue;
                            }
                        }
                    }
                    // Location отсутствует или неверен
                    break;
                } else {
                    result.status_code = Some(status);
                    result.final_url = Some(current_url);
                    // Получаем размер
                    if let Ok(bytes) = resp.bytes().await {
                        result.size = bytes.len() as u64;
                    }
                    break;
                }
            }
            Err(e) => {
                result.error = Some(e.to_string());
                break;
            }
        }
    }

    if redirect_count > max_redirects && result.error.is_none() {
        result.error = Some(format!("Превышен лимит редиректов ({})", max_redirects));
    }

    result.time = start.elapsed().as_secs_f64();
    result
}

async fn check_multiple(urls: Vec<String>, client: Client, max_redirects: usize, workers: usize) -> HashMap<String, Result> {
    use futures::future::join_all;
    let mut results = HashMap::new();

    let chunks: Vec<_> = urls.chunks(workers).collect();
    for chunk in chunks {
        let tasks: Vec<_> = chunk.iter().map(|url| check_url(&client, url, max_redirects)).collect();
        let chunk_results = join_all(tasks).await;
        for (url, res) in chunk.iter().zip(chunk_results) {
            results.insert(url.clone(), res);
        }
    }
    results
}

fn print_results(results: &HashMap<String, Result>) {
    for (url, res) in results {
        println!("\n{}", format!("🔍 Проверка: {}", url).cyan());
        if let Some(err) = &res.error {
            println!("{}", format!("❌ Ошибка: {}", err).red());
            continue;
        }
        let status_color = match res.status_code {
            Some(200..=299) => "green",
            Some(300..=399) => "yellow",
            Some(_) => "red",
            None => "red",
        };
        let status_text = match res.status_code {
            Some(code) if (200..300).contains(&code) => format!("{} OK", code),
            Some(code) if (300..400).contains(&code) => format!("{} Redirect", code),
            Some(code) => format!("{} Error", code),
            None => "N/A".to_string(),
        };
        let final_url = res.final_url.as_deref().unwrap_or(&url);
        println!("{}", format!("✅ {}", final_url).color(status_color));
        println!("   Статус: {}", status_text.color(status_color));
        println!("   Время: {:.2} сек", res.time);
        println!("   Размер: {} байт", res.size);
        if !res.redirects.is_empty() {
            println!("   Редиректы: {}", res.redirects.len());
            for (i, step) in res.redirects.iter().enumerate() {
                println!("     {}. {} -> {}", i+1, step.url, step.status_code);
            }
        }
    }
}

fn save_json(results: &HashMap<String, Result>, filename: &str) -> io::Result<()> {
    let json = serde_json::to_string_pretty(results)?;
    let mut file = File::create(filename)?;
    file.write_all(json.as_bytes())?;
    println!("{}", format!("💾 Сохранено JSON: {}", filename).green());
    Ok(())
}

fn save_csv(results: &HashMap<String, Result>, filename: &str) -> io::Result<()> {
    let mut file = File::create(filename)?;
    writeln!(file, "URL,Final URL,Status,Time (s),Size (bytes),Redirects,Error")?;
    for (url, res) in results {
        writeln!(file, "{},{},{},{:.3},{},{},{}",
            url,
            res.final_url.as_deref().unwrap_or(""),
            res.status_code.unwrap_or(0),
            res.time,
            res.size,
            res.redirects.len(),
            res.error.as_deref().unwrap_or("")
        )?;
    }
    println!("{}", format!("💾 Сохранено CSV: {}", filename).green());
    Ok(())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let matches = App::new("Link Checker (Redirects)")
        .arg(Arg::with_name("urls").multiple(true).help("Список URL"))
        .arg(Arg::with_name("file").short('f').long("file").takes_value(true).help("Файл со списком URL"))
        .arg(Arg::with_name("timeout").short('t').long("timeout").takes_value(true).default_value("10").help("Таймаут (сек)"))
        .arg(Arg::with_name("max-redirects").short('m').long("max-redirects").takes_value(true).default_value("10").help("Максимум редиректов"))
        .arg(Arg::with_name("output-json").long("output-json").takes_value(true).help("Сохранить в JSON"))
        .arg(Arg::with_name("output-csv").long("output-csv").takes_value(true).help("Сохранить в CSV"))
        .arg(Arg::with_name("workers").short('w').long("workers").takes_value(true).default_value("10").help("Количество потоков"))
        .arg(Arg::with_name("header").short('H').long("header").takes_value(true).multiple(true).help("Заголовки (key:value)"))
        .get_matches();

    println!("{}", "🔗 Link Checker (Rust)".cyan());

    let mut urls: Vec<String> = Vec::new();
    if let Some(urls_arg) = matches.values_of("urls") {
        urls.extend(urls_arg.map(|s| s.to_string()));
    }
    if let Some(file_path) = matches.value_of("file") {
        let file = File::open(file_path)?;
        let reader = io::BufReader::new(file);
        for line in reader.lines() {
            let l = line?;
            let trimmed = l.trim();
            if !trimmed.is_empty() {
                urls.push(trimmed.to_string());
            }
        }
    }
    if urls.is_empty() {
        println!("❌ Нет URL для проверки.");
        std::process::exit(1);
    }

    let timeout = matches.value_of("timeout").unwrap().parse::<u64>()?;
    let max_redirects = matches.value_of("max-redirects").unwrap().parse::<usize>()?;
    let workers = matches.value_of("workers").unwrap().parse::<usize>()?;

    let mut headers = reqwest::header::HeaderMap::new();
    if let Some(header_values) = matches.values_of("header") {
        for h in header_values {
            if let Some((key, value)) = h.split_once(':') {
                headers.insert(key.trim(), value.trim().parse()?);
            }
        }
    }

    let client = Client::builder()
        .timeout(std::time::Duration::from_secs(timeout))
        .default_headers(headers)
        .build()?;

    let results = check_multiple(urls, client, max_redirects, workers).await;
    print_results(&results);

    if let Some(json_file) = matches.value_of("output-json") {
        save_json(&results, json_file)?;
    }
    if let Some(csv_file) = matches.value_of("output-csv") {
        save_csv(&results, csv_file)?;
    }

    Ok(())
}
