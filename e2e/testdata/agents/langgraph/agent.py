from typing import TypedDict
from langchain_openai import ChatOpenAI
from langgraph.graph import StateGraph, START, END
from loop import serve, MODEL_URL, TOKEN

llm = ChatOpenAI(model="gpt-4o-mini", base_url=MODEL_URL, api_key=TOKEN)


class State(TypedDict):
    question: str
    answer: str


def think(state: State) -> State:
    return {"question": state["question"], "answer": llm.invoke(state["question"]).content}


graph = StateGraph(State)
graph.add_node("think", think)
graph.add_edge(START, "think")
graph.add_edge("think", END)
app = graph.compile()

serve(lambda body: app.invoke({"question": body, "answer": ""})["answer"])
