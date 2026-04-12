import asyncio
from ollama import AsyncClient
async def main():
    client = AsyncClient(host='http://192.168.1.2:11434')
    response = await client.chat(model='llama3', messages=[{'role': 'user', 'content': 'Hi'}], stream=False)
    print(response)

asyncio.run(main())
