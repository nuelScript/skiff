FROM python:3.12-slim
RUN pip install --no-cache-dir dagster==1.13.26 dagster-webserver==1.13.26 dagster-postgres==0.29.26
ENV DAGSTER_HOME=/opt/dagster
WORKDIR /app
COPY dagster.yaml /opt/dagster/
COPY start.sh defs.py ./
ENTRYPOINT ["/app/start.sh"]
CMD ["dagster-webserver", "-h", "0.0.0.0", "-p", "3000", "-m", "defs"]
