from crewai import Agent, Crew, LLM, Task
from loop import serve, MODEL_URL, TOKEN

llm = LLM(model="openai/gpt-4o-mini", base_url=MODEL_URL, api_key=TOKEN)


def answer(body):
    helper = Agent(role="Helper", goal="Answer the request", backstory="You help.", llm=llm, allow_delegation=False)
    task = Task(description=body, expected_output="A short answer", agent=helper)
    return str(Crew(agents=[helper], tasks=[task]).kickoff())


serve(answer)
