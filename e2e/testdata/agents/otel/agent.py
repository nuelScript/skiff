# A plain OpenAI agent with OpenTelemetry: Skiff sets the exporter's address and token, and the model call carries the run's trace.
from openai import OpenAI
from opentelemetry import propagate, trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from loop import serve, MODEL_URL, TOKEN

provider = TracerProvider()
provider.add_span_processor(SimpleSpanProcessor(OTLPSpanExporter()))
trace.set_tracer_provider(provider)
tracer = trace.get_tracer("e2e-agent")
client = OpenAI(base_url=MODEL_URL, api_key=TOKEN)


def answer(body):
    with tracer.start_as_current_span("handle event"):
        headers = {}
        propagate.inject(headers)
        reply = client.chat.completions.create(model="gpt-4o-mini", messages=[{"role": "user", "content": body}], extra_headers=headers)
        return reply.choices[0].message.content


serve(answer)
