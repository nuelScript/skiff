import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Serve the install scripts at clean paths, as plain text:
  //   /install → the server control-plane installer
  //   /cli     → the CLI installer for your own machine
  async rewrites() {
    return [
      { source: "/install", destination: "/install.sh" },
      { source: "/cli", destination: "/cli.sh" },
    ];
  },
  async headers() {
    const plain = [{ key: "Content-Type", value: "text/plain; charset=utf-8" }];
    return [
      { source: "/install", headers: plain },
      { source: "/install.sh", headers: plain },
      { source: "/cli", headers: plain },
      { source: "/cli.sh", headers: plain },
    ];
  },
};

export default nextConfig;
