require "socket"

port = ENV["PORT"] || "8080"
server = TCPServer.new("0.0.0.0", port.to_i)
puts "ruby-hello listening on #{port}"

loop do
  client = server.accept
  body = "hello from skiff (ruby, no Dockerfile)"
  client.write "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: #{body.bytesize}\r\n\r\n#{body}"
  client.close
end
