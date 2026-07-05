import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Serve the install script at the clean /install path, as plain text.
  async rewrites() {
    return [{ source: "/install", destination: "/install.sh" }];
  },
  async headers() {
    const plain = [{ key: "Content-Type", value: "text/plain; charset=utf-8" }];
    return [
      { source: "/install", headers: plain },
      { source: "/install.sh", headers: plain },
    ];
  },
};

export default nextConfig;
