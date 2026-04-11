from tests.fakes.tool_helpers import create_tool_executor
import asyncio

async def main():
    executor = create_tool_executor()
    svc = executor.operator_command_service
    print("Has _store:", hasattr(svc, '_store'))

asyncio.run(main())
