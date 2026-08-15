# link_checker.rb — Ruby версия

require 'http'
require 'json'
require 'csv'
require 'optparse'
require 'concurrent'
require 'colorize'

class LinkChecker
  def initialize(timeout: 10, max_redirects: 10, headers: {})
    @timeout = timeout
    @max_redirects = max_redirects
    @headers = headers
  end

  def check_url(url)
    start = Time.now
    result = {
      url: url,
      final_url: nil,
      status_code: nil,
      redirects: [],
      time: 0,
      size: 0,
      error: nil
    }

    current_url = url
    redirect_count = 0

    begin
      while redirect_count <= @max_redirects
        response = HTTP.headers(@headers)
                      .timeout(@timeout)
                      .follow(false)
                      .get(current_url)

        status = response.status.to_i
        result[:redirects] << {
          url: current_url,
          status_code: status,
          headers: response.headers.to_h
        }

        if (300..399).cover?(status)
          location = response.headers['Location']
          break unless location
          # Разрешаем относительный URL
          current_url = URI.join(current_url, location).to_s
          redirect_count += 1
          if redirect_count > @max_redirects
            result[:error] = "Слишком много редиректов (> #{@max_redirects})"
            break
          end
        else
          result[:status_code] = status
          result[:final_url] = current_url
          result[:size] = response.body.to_s.bytesize
          break
        end
      end
      if redirect_count > @max_redirects && result[:error].nil?
        result[:error] = "Превышен лимит редиректов (#{@max_redirects})"
      end
    rescue => e
      result[:error] = e.message
    end

    result[:time] = Time.now - start
    result
  end

  def check_multiple(urls, workers: 10)
    results = {}
    pool = Concurrent::FixedThreadPool.new(workers)
    futures = urls.map do |url|
      Concurrent::Future.execute(executor: pool) do
        [url, check_url(url)]
      end
    end
    futures.each { |f| url, res = f.value; results[url] = res }
    pool.shutdown
    pool.wait_for_termination
    results
  end
end

def print_results(results)
  results.each do |url, res|
    puts "\n🔍 Проверка: #{url}".cyan
    if res[:error]
      puts "❌ Ошибка: #{res[:error]}".red
      next
    end
    status_color = case res[:status_code]
    when 200..299 then :green
    when 300..399 then :yellow
    else :red
    end
    status_text = case res[:status_code]
    when 200..299 then "#{res[:status_code]} OK"
    when 300..399 then "#{res[:status_code]} Redirect"
    else "#{res[:status_code]} Error"
    end
    puts "#{'✅'.colorize(status_color)} #{res[:final_url]}".colorize(status_color)
    puts "   Статус: #{status_text.colorize(status_color)}"
    puts "   Время: #{'%.2f' % res[:time]} сек"
    puts "   Размер: #{res[:size]} байт"
    unless res[:redirects].empty?
      puts "   Редиректы: #{res[:redirects].size}"
      res[:redirects].each_with_index do |r, i|
        puts "     #{i+1}. #{r[:url]} -> #{r[:status_code]}"
      end
    end
  end
end

def save_json(results, filename)
  File.write(filename, JSON.pretty_generate(results))
  puts "💾 Сохранено JSON: #{filename}".green
end

def save_csv(results, filename)
  CSV.open(filename, 'w') do |csv|
    csv << ['URL', 'Final URL', 'Status', 'Time (s)', 'Size (bytes)', 'Redirects', 'Error']
    results.each do |url, res|
      csv << [
        url,
        res[:final_url] || '',
        res[:status_code] || '',
        '%.3f' % res[:time],
        res[:size] || 0,
        res[:redirects].size,
        res[:error] || ''
      ]
    end
  end
  puts "💾 Сохранено CSV: #{filename}".green
end

def main
  options = { timeout: 10, max_redirects: 10, workers: 10 }
  OptionParser.new do |opts|
    opts.banner = "Usage: ruby link_checker.rb [options] [url...]"
    opts.on("-f FILE", "--file FILE", "Файл со списком URL") { |v| options[:file] = v }
    opts.on("-t SECONDS", "--timeout SECONDS", Integer, "Таймаут") { |v| options[:timeout] = v }
    opts.on("-m N", "--max-redirects N", Integer, "Максимум редиректов") { |v| options[:max_redirects] = v }
    opts.on("-o FILE", "--output-json FILE", "Сохранить в JSON") { |v| options[:output_json] = v }
    opts.on("-c FILE", "--output-csv FILE", "Сохранить в CSV") { |v| options[:output_csv] = v }
    opts.on("-w N", "--workers N", Integer, "Количество потоков") { |v| options[:workers] = v }
    opts.on("-H HEADER", "--header HEADER", "Заголовки (key:value)") { |v| (options[:headers] ||= []) << v }
  end.parse!

  urls = ARGV

  if options[:file]
    File.readlines(options[:file]).each do |line|
      line.strip!
      urls << line unless line.empty?
    end
  end

  if urls.empty?
    puts "❌ Нет URL для проверки."
    exit 1
  end

  puts "🔗 Link Checker (Ruby)".cyan

  headers = {}
  if options[:headers]
    options[:headers].each do |h|
      key, value = h.split(':', 2)
      headers[key.strip] = value.strip if key && value
    end
  end

  checker = LinkChecker.new(
    timeout: options[:timeout],
    max_redirects: options[:max_redirects],
    headers: headers
  )
  results = checker.check_multiple(urls, workers: options[:workers])
  print_results(results)

  save_json(results, options[:output_json]) if options[:output_json]
  save_csv(results, options[:output_csv]) if options[:output_csv]
end

main if __FILE__ == $0
