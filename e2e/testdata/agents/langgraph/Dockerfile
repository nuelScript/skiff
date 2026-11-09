FROM python:3.12-slim
ENV PYTHONUNBUFFERED=1 CREWAI_DISABLE_TELEMETRY=true
RUN pip install --no-cache-dir requests langgraph==0.2.60 langchain-openai==0.2.14
WORKDIR /app
COPY loop.py agent.py ./
CMD ["python", "agent.py"]
