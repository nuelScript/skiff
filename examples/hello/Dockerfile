FROM python:3-alpine
WORKDIR /app
RUN printf '<!doctype html><title>skiff</title><h1>hello from skiff</h1>' > index.html
EXPOSE 8080
CMD ["python", "-m", "http.server", "8080"]
