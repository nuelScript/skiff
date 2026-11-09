FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 CREWAI_DISABLE_TELEMETRY=true
RUN pip install --no-cache-dir requests openai==1.58.1 opentelemetry-sdk==1.29.0 opentelemetry-exporter-otlp-proto-http==1.29.0
WORKDIR /app
COPY loop.py agent.py ./
CMD ["python", "agent.py"]
