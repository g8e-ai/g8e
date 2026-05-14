# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import pytest
import shutil
import tempfile
from unittest.mock import AsyncMock, MagicMock, patch

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec

from app.services.auth.certificate_service import CertificateService
from app.services.auth.certificate_data_service import CertificateDataService

@pytest.fixture
def temp_pki_dir():
    path = tempfile.mkdtemp()
    yield path
    shutil.rmtree(path)

@pytest.fixture
def ca_key():
    return ec.generate_private_key(ec.SECP384R1())

@pytest.fixture
def ca_cert(ca_key):
    subject = issuer = x509.Name([
        x509.NameAttribute(x509.NameOID.COMMON_NAME, u"g8e Test CA"),
    ])
    builder = x509.CertificateBuilder().subject_name(
        subject
    ).issuer_name(
        issuer
    ).public_key(
        ca_key.public_key()
    ).serial_number(
        x509.random_serial_number()
    ).not_valid_before(
        pytest.importorskip("datetime").datetime.now(pytest.importorskip("datetime").UTC)
    ).not_valid_after(
        pytest.importorskip("datetime").datetime.now(pytest.importorskip("datetime").UTC) + pytest.importorskip("datetime").timedelta(days=10)
    ).add_extension(
        x509.BasicConstraints(ca=True, path_length=None), critical=True,
    )
    return builder.sign(ca_key, hashes.SHA256())

@pytest.fixture
def setup_ca_files(temp_pki_dir, ca_cert):
    trust_dir = os.path.join(temp_pki_dir, "trust")
    os.makedirs(trust_dir, exist_ok=True)
    cert_path = os.path.join(trust_dir, "hub-bundle.pem")
    
    with open(cert_path, "wb") as f:
        f.write(ca_cert.public_bytes(serialization.Encoding.PEM))
    
    return temp_pki_dir

@pytest.fixture
def mock_data_service():
    service = MagicMock(spec=CertificateDataService)
    service.get_all_revocations = AsyncMock(return_value=[])
    service.revoke_certificate = AsyncMock(return_value=True)
    
    # Mock the internal HTTP client structure used in generate_operator_certificate
    mock_db_client = AsyncMock()
    mock_db_client._request_json.return_value = {
        "success": True,
        "certificate_pem": "-----BEGIN CERTIFICATE-----\nMOCK OPERATOR CERT\n-----END CERTIFICATE-----",
        "serial": "MOCK-SERIAL-ABC123DEF"
    }
    
    # Properly nest mocks
    service.cache = MagicMock()
    service.cache.db = MagicMock()
    service.cache.db.client = mock_db_client
    
    return service

@pytest.mark.asyncio
async def test_initialize_success(setup_ca_files, mock_data_service):
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    await service.initialize()
    
    assert service.initialized is True
    assert service.ca_cert is not None
    # service.ca_key should be None now as it's not loaded
    assert service.ca_key is None
    mock_data_service.get_all_revocations.assert_called_once()

@pytest.mark.asyncio
async def test_initialize_alternate_path(temp_pki_dir, ca_cert, mock_data_service):
    # Test path: pki_dir/authorities/hub_ca.crt
    auth_subdir = os.path.join(temp_pki_dir, "authorities")
    os.makedirs(auth_subdir)
    
    cert_path = os.path.join(auth_subdir, "hub_ca.crt")
    
    with open(cert_path, "wb") as f:
        f.write(ca_cert.public_bytes(serialization.Encoding.PEM))
        
    service = CertificateService(pki_dir=temp_pki_dir, data_service=mock_data_service)
    await service.initialize()
    
    assert service.initialized is True
    assert service.ca_cert is not None

@pytest.mark.asyncio
async def test_initialize_with_revocations(setup_ca_files, mock_data_service):
    mock_data_service.get_all_revocations.return_value = [
        {"serial": "ABC12345"},
        {"serial": "DEF67890"}
    ]
    
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    await service.initialize()
    
    assert service.is_revoked("abc12345") is True
    assert service.is_revoked("DEF67890") is True
    assert service.is_revoked("123") is False

@pytest.mark.asyncio
async def test_initialize_fails_no_files(temp_pki_dir, mock_data_service):
    service = CertificateService(pki_dir=temp_pki_dir, data_service=mock_data_service)
    # This shouldn't raise, but should log and leave initialized=False
    await service.initialize()
    assert service.initialized is False

@pytest.mark.asyncio
async def test_generate_operator_certificate(setup_ca_files, mock_data_service):
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    await service.initialize()
    
    res = await service.generate_operator_certificate(
        operator_id="test-op",
        user_id="test-user",
        organization_id="test-org"
    )
    
    assert "cert" in res
    assert "key" in res
    assert "serial" in res
    assert res["cert"] == "-----BEGIN CERTIFICATE-----\nMOCK OPERATOR CERT\n-----END CERTIFICATE-----"
    assert res["serial"] == "MOCK-SERIAL-ABC123DEF"
    
    # Verify the generated key is valid PEM (local generation)
    assert "-----BEGIN PRIVATE KEY-----" in res["key"]
    
    # Verify the signing request was made
    mock_data_service.cache.db.client._request_json.assert_called_once()
    call_args = mock_data_service.cache.db.client._request_json.call_args
    assert call_args[0][0] == "POST"
    assert call_args[0][1] == "/.well-known/g8e/pki/sign-csr"
    payload = call_args[1]["json"]
    assert payload["common_name"] == "test-op"
    assert payload["organizational_unit"] == "test-user"

@pytest.mark.asyncio
async def test_generate_without_initialize_triggers_init(setup_ca_files, mock_data_service):
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    # Don't call initialize() explicitly
    
    res = await service.generate_operator_certificate(
        operator_id="test-op",
        user_id="test-user",
        organization_id="test-org"
    )
    assert service.initialized is True
    assert "cert" in res

@pytest.mark.asyncio
async def test_generate_even_if_no_ca_cert(temp_pki_dir, mock_data_service):
    # If ca.crt is missing, it logs an error but doesn't prevent signing via API
    service = CertificateService(pki_dir=temp_pki_dir, data_service=mock_data_service)
    res = await service.generate_operator_certificate("op", "user", "org")
    assert "cert" in res

@pytest.mark.asyncio
async def test_revoke_certificate(setup_ca_files, mock_data_service):
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    await service.initialize()
    
    serial = "ABC123DEF"
    success = await service.revoke_certificate(serial, reason="test", operator_id="op1")
    
    assert success is True
    assert service.is_revoked(serial) is True
    mock_data_service.revoke_certificate.assert_called_once_with(serial.upper(), "test", "op1")

def test_get_crl(mock_data_service):
    service = CertificateService(data_service=mock_data_service)
    service._revoked_serials.add("SERIAL1")
    service._revoked_serials.add("SERIAL2")
    
    crl = service.get_crl()
    assert crl["version"] == 1
    serials = [r["serial"] for r in crl["revoked_certificates"]]
    assert "SERIAL1" in serials
    assert "SERIAL2" in serials

@pytest.mark.asyncio
async def test_cleanup(setup_ca_files, mock_data_service):
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    await service.initialize()
    service._revoked_serials.add("S1")
    
    await service.cleanup()
    
    assert service.ca_cert is None
    assert service.ca_key is None
    assert len(service._revoked_serials) == 0
    assert service.initialized is False

@pytest.mark.asyncio
async def test_initialize_already_initialized(setup_ca_files, mock_data_service):
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    service.initialized = True
    await service.initialize()
    # Should return early and NOT call data_service again
    mock_data_service.get_all_revocations.assert_not_called()

@pytest.mark.asyncio
async def test_initialize_no_data_service(setup_ca_files):
    service = CertificateService(pki_dir=setup_ca_files, data_service=None)
    await service.initialize()
    assert service.initialized is True
    assert service.ca_cert is not None

@pytest.mark.asyncio
async def test_initialize_revocation_missing_serial(setup_ca_files, mock_data_service):
    mock_data_service.get_all_revocations.return_value = [
        {"reason": "no serial"}
    ]
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    await service.initialize()
    assert service.initialized is True
    assert len(service._revoked_serials) == 0

@pytest.mark.asyncio
async def test_initialize_data_service_exception(setup_ca_files, mock_data_service):
    mock_data_service.get_all_revocations.side_effect = Exception("DB Down")
    
    service = CertificateService(pki_dir=setup_ca_files, data_service=mock_data_service)
    # Should not raise, just log error
    await service.initialize()
    assert service.initialized is True
    assert len(service._revoked_serials) == 0

@pytest.mark.asyncio
async def test_revoke_no_data_service(setup_ca_files):
    service = CertificateService(pki_dir=setup_ca_files, data_service=None)
    await service.initialize()
    
    serial = "XYZ987"
    await service.revoke_certificate(serial, "test")
    assert service.is_revoked(serial) is True
