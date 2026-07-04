import com.sun.net.httpserver.HttpServer;
import java.io.OutputStream;
import java.net.InetSocketAddress;

public class Main {
    public static void main(String[] args) throws Exception {
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8080"));
        HttpServer server = HttpServer.create(new InetSocketAddress("0.0.0.0", port), 0);
        server.createContext("/", exchange -> {
            byte[] body = "hello from skiff (java, no Dockerfile)".getBytes();
            exchange.sendResponseHeaders(200, body.length);
            OutputStream os = exchange.getResponseBody();
            os.write(body);
            os.close();
        });
        server.start();
        System.out.println("java-hello listening on " + port);
    }
}
