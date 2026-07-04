defmodule Hello.MixProject do
  use Mix.Project

  def project do
    [app: :hello, version: "0.1.0", elixir: "~> 1.15"]
  end

  def application do
    [mod: {Hello.Application, []}, extra_applications: [:logger]]
  end
end
