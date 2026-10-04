# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""The Gateway's signed browser-proxy identity stamp, verified by g8ee."""

import asyncio
import hashlib

import pytest
from nacl.signing import SigningKey

from app.constants import (
    X_PROXY_ISSUED_AT,
    X_PROXY_ORGANIZATION_ID,
    X_PROXY_USER_EMAIL,
    X_PROXY_USER_ID,
)
from app.errors import AuthenticationError
from app.models.auth import ProxySigningKeyResponse
from app.services.auth.proxy_stamp import BrowserProxyStamp
from proxy_stamp_support import (
    NOW,
    VECTORS,
    Clock,
    KEY_ID,
    KeySource,
    key_response as _response,
    make_key as _key,
    make_request,
    stamped_headers,
    verifier,
)


class TestSharedConformanceVector:
    def test_canonical_bytes_match_the_gateway(self):
        stamp = BrowserProxyStamp(**VECTORS["stamp"])
        assert stamp.canonical_bytes().hex() == VECTORS["canonical_bytes_hex"]

    def test_gateway_signature_verifies_against_the_gateway_public_key(self):
        key = SigningKey(bytes.fromhex(VECTORS["private_seed_hex"]))
        assert bytes(key.verify_key).hex() == VECTORS["public_key_hex"]
        assert key.sign(bytes.fromhex(VECTORS["canonical_bytes_hex"])).signature.hex() == VECTORS["signature_hex"]

    def test_vector_body_hash_is_the_hash_of_the_vector_body(self):
        assert hashlib.sha256(VECTORS["body_utf8"].encode()).hexdigest() == VECTORS["stamp"]["body_sha256"]

    def test_fields_with_line_breaks_are_refused(self):
        stamp = BrowserProxyStamp(**{**VECTORS["stamp"], "user_id": "user-1\nweb-2"})
        with pytest.raises(AuthenticationError):
            stamp.canonical_bytes()


class TestProxyStampVerifier:
    @pytest.mark.asyncio
    async def test_valid_signed_request_is_accepted(self):
        key = _key()
        await verifier(KeySource(_response(key))).verify(make_request(stamped_headers(key)))

    @pytest.mark.asyncio
    async def test_request_without_a_stamp_is_rejected(self):
        headers = {X_PROXY_USER_ID: "user-1", X_PROXY_USER_EMAIL: "user-1@g8e.local"}
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(_key()))).verify(make_request(headers))

    @pytest.mark.asyncio
    async def test_old_static_marker_is_not_a_stamp(self):
        headers = {
            "X-G8E-Gateway-Browser-Proxy": "1",
            X_PROXY_USER_ID: "user-1",
            X_PROXY_USER_EMAIL: "user-1@g8e.local",
        }
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(_key()))).verify(make_request(headers))

    @pytest.mark.asyncio
    async def test_signature_from_another_key_is_rejected(self):
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(_key(1)))).verify(make_request(stamped_headers(_key(2))))

    @pytest.mark.asyncio
    async def test_tampered_body_is_rejected(self):
        key = _key()
        request = make_request(stamped_headers(key), body=b'{"message":"rm -rf /"}')
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(request)

    @pytest.mark.asyncio
    async def test_tampered_path_is_rejected(self):
        key = _key()
        request = make_request(stamped_headers(key), path="/api/v1/settings/llm")
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(request)

    @pytest.mark.asyncio
    async def test_tampered_query_is_rejected(self):
        key = _key()
        request = make_request(stamped_headers(key), query="stream=0")
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(request)

    @pytest.mark.asyncio
    async def test_tampered_method_is_rejected(self):
        key = _key()
        request = make_request(stamped_headers(key), method="PUT")
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(request)

    @pytest.mark.asyncio
    async def test_swapped_user_identity_is_rejected(self):
        key = _key()
        headers = stamped_headers(key)
        headers[X_PROXY_USER_ID] = "admin"
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(make_request(headers))

    @pytest.mark.asyncio
    async def test_unsigned_organization_header_is_rejected(self):
        key = _key()
        headers = stamped_headers(key)
        headers[X_PROXY_ORGANIZATION_ID] = "org-forged"
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(make_request(headers))

    @pytest.mark.asyncio
    @pytest.mark.parametrize("offset", [-31, 31, -3600])
    async def test_stale_or_future_timestamp_is_rejected(self, offset):
        key = _key()
        request = make_request(stamped_headers(key, issued_at=int(NOW) + offset))
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(request)

    @pytest.mark.asyncio
    async def test_timestamp_at_the_edge_of_the_window_is_accepted(self):
        key = _key()
        await verifier(KeySource(_response(key))).verify(make_request(stamped_headers(key, issued_at=int(NOW) - 30)))

    @pytest.mark.asyncio
    async def test_non_numeric_timestamp_is_rejected(self):
        key = _key()
        headers = stamped_headers(key)
        headers[X_PROXY_ISSUED_AT] = "yesterday"
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(_response(key))).verify(make_request(headers))

    @pytest.mark.asyncio
    async def test_replayed_nonce_is_rejected(self):
        key = _key()
        v = verifier(KeySource(_response(key)))
        headers = stamped_headers(key)
        await v.verify(make_request(headers))
        with pytest.raises(AuthenticationError):
            await v.verify(make_request(headers))

    @pytest.mark.asyncio
    async def test_nonce_is_forgotten_after_the_window(self):
        key = _key()
        clock = Clock()
        v = verifier(KeySource(_response(key)), clock)
        await v.verify(make_request(stamped_headers(key)))
        clock.now = NOW + 61
        await v.verify(make_request(stamped_headers(key, issued_at=int(clock.now))))
        clock.now = NOW + 62
        with pytest.raises(AuthenticationError):
            await v.verify(make_request(stamped_headers(key, issued_at=int(clock.now), nonce="n" * 32)))

    @pytest.mark.asyncio
    async def test_failed_signature_does_not_burn_the_nonce(self):
        key = _key()
        v = verifier(KeySource(_response(key)))
        bad = stamped_headers(key)
        with pytest.raises(AuthenticationError):
            await v.verify(make_request(bad, body=b"tampered"))
        await v.verify(make_request(bad))

    @pytest.mark.asyncio
    async def test_full_nonce_cache_rejects_instead_of_forgetting(self):
        key = _key()
        v = verifier(KeySource(_response(key)), max_nonces=2)
        await v.verify(make_request(stamped_headers(key, nonce="a" * 32)))
        await v.verify(make_request(stamped_headers(key, nonce="b" * 32)))
        with pytest.raises(AuthenticationError):
            await v.verify(make_request(stamped_headers(key, nonce="c" * 32)))

    @pytest.mark.asyncio
    async def test_unreachable_gateway_key_endpoint_fails_closed(self):
        key = _key()
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(RuntimeError("gateway down"))).verify(make_request(stamped_headers(key)))

    @pytest.mark.asyncio
    async def test_non_ed25519_key_from_gateway_fails_closed(self):
        key = _key()
        bad = ProxySigningKeyResponse(key_id=KEY_ID, public_key=bytes(key.verify_key).hex(), algorithm="rsa")
        with pytest.raises(AuthenticationError):
            await verifier(KeySource(bad)).verify(make_request(stamped_headers(key)))

    @pytest.mark.asyncio
    async def test_key_is_fetched_once_and_cached(self):
        key = _key()
        source = KeySource(_response(key))
        v = verifier(source)
        await v.verify(make_request(stamped_headers(key, nonce="a" * 32)))
        await v.verify(make_request(stamped_headers(key, nonce="b" * 32)))
        assert source.calls == 1

    @pytest.mark.asyncio
    async def test_rotated_gateway_key_is_picked_up_by_key_id(self):
        old, new = _key(1), _key(2)
        source = KeySource(_response(old, "k1"), _response(new, "k2"))
        clock = Clock()
        v = verifier(source, clock)
        await v.verify(make_request(stamped_headers(old, key_id="k1", nonce="a" * 32)))
        clock.now = NOW + 11
        await v.verify(make_request(stamped_headers(new, key_id="k2", nonce="b" * 32, issued_at=int(clock.now))))
        assert source.calls == 2

    @pytest.mark.asyncio
    async def test_unknown_key_ids_cannot_force_a_fetch_per_request(self):
        key = _key()
        source = KeySource(_response(key))
        v = verifier(source)
        await v.verify(make_request(stamped_headers(key, nonce="a" * 32)))
        for i in range(5):
            with pytest.raises(AuthenticationError):
                await v.verify(make_request(stamped_headers(key, key_id=f"bogus-{i}", nonce=f"{i}" * 32)))
        assert source.calls == 1

    @pytest.mark.asyncio
    async def test_concurrent_requests_on_cold_cache_share_one_fetch(self):
        # Browser fan-out on first console load: every request must wait for
        # the in-flight fetch, not trip the refetch throttle and 401.
        key = _key()
        source = KeySource(_response(key), delay=0.01)
        v = verifier(source)
        await asyncio.gather(*(v.verify(make_request(stamped_headers(key, nonce=c * 32))) for c in "abc"))
        assert source.calls == 1

    @pytest.mark.asyncio
    async def test_concurrent_requests_during_key_rotation_share_one_fetch(self):
        old, new = _key(1), _key(2)
        source = KeySource(_response(old, "k1"), _response(new, "k2"), delay=0.01)
        clock = Clock()
        v = verifier(source, clock)
        await v.verify(make_request(stamped_headers(old, key_id="k1", nonce="a" * 32)))
        clock.now = NOW + 11
        await asyncio.gather(
            *(
                v.verify(make_request(stamped_headers(new, key_id="k2", nonce=c * 32, issued_at=int(clock.now))))
                for c in "bcd"
            )
        )
        assert source.calls == 2

    @pytest.mark.asyncio
    async def test_concurrent_requests_fail_closed_when_the_shared_fetch_fails(self):
        key = _key()
        source = KeySource(RuntimeError("gateway down"), delay=0.01)
        v = verifier(source)
        results = await asyncio.gather(
            *(v.verify(make_request(stamped_headers(key, nonce=c * 32))) for c in "abc"),
            return_exceptions=True,
        )
        assert all(isinstance(r, AuthenticationError) for r in results)
        assert source.calls == 1
