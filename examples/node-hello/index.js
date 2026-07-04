const http = require("http");

const port = process.env.PORT || 3000;

http
  .createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end("<!doctype html><title>skiff</title><h1>hello from skiff (node, no Dockerfile)</h1>");
  })
  .listen(port, () => console.log(`node-hello listening on ${port}`));
