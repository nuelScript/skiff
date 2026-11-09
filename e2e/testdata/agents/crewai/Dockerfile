FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 CREWAI_DISABLE_TELEMETRY=true
RUN pip install --no-cache-dir requests crewai==0.86.0
WORKDIR /app
COPY loop.py agent.py ./
CMD ["python", "agent.py"]
