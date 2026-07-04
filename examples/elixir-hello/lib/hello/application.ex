defmodule Hello.Application do
  use Application

  def start(_type, _args) do
    port = String.to_integer(System.get_env("PORT") || "8080")
    {:ok, socket} = :gen_tcp.listen(port, [:binary, packet: :raw, active: false, reuseaddr: true])
    IO.puts("elixir-hello listening on #{port}")
    spawn(fn -> accept_loop(socket) end)
    Supervisor.start_link([], strategy: :one_for_one)
  end

  defp accept_loop(socket) do
    {:ok, client} = :gen_tcp.accept(socket)
    body = "hello from skiff (elixir, no Dockerfile)"
    resp = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: #{byte_size(body)}\r\n\r\n#{body}"
    :gen_tcp.send(client, resp)
    :gen_tcp.close(client)
    accept_loop(socket)
  end
end
