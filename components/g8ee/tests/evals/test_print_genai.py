import pytest
from app.services.ai.grounding.web_search_provider import WebSearchProvider
import gc
import aiohttp

@pytest.mark.asyncio
async def test_search():
    p = WebSearchProvider("foo", "bar", "123")
    try:
        await p.search("query")
    except Exception as e:
        print("error:", type(e))
    gc.collect()
    sessions = [obj for obj in gc.get_objects() if isinstance(obj, aiohttp.ClientSession)]
    print("AIOHTTP SESSIONS COUNT:", len(sessions))
    for s in sessions:
        print("SESSION:", s, "CLOSED:", s.closed)
