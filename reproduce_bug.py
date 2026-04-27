from pydantic import Field, ValidationError
import sys
import os

# Add the app directory to sys.path to import models
sys.path.append(os.path.join(os.getcwd(), "components/g8ee"))

from app.models.internal_api import OperatorAuthenticateRequest

def test_extra_fields():
    print("Testing OperatorAuthenticateRequest with extra fields...")
    payload = {
        "system_info": {"os": "linux"},
        "authorization_header": "Bearer some_key" # This is the extra field g8ed sends
    }
    
    try:
        request = OperatorAuthenticateRequest(**payload)
        print("Successfully created request with extra fields (this is the BUG)")
        print(f"Request data: {request.model_dump()}")
        if "authorization_header" not in request.model_dump():
            print("Field 'authorization_header' was silently dropped.")
    except ValidationError as e:
        print("ValidationError raised as expected (Bug Fixed!):")
        print(e)

if __name__ == "__main__":
    test_extra_fields()
