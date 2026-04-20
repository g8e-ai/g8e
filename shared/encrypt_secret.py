#!/usr/bin/env python3

import sys
import base64
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

def encrypt_secret(plaintext: str, key_hex: str) -> str:
    if not key_hex:
        return plaintext
    
    # Convert hex key to bytes
    key = bytes.fromhex(key_hex)
    if len(key) != 32:
        raise ValueError("Encryption key must be 32 bytes (64 hex chars)")
    
    # Encrypt using AES-256-GCM
    aesgcm = AESGCM(key)
    nonce = AESGCM.generate_nonce(12)  # 12 bytes for GCM
    ciphertext = aesgcm.encrypt(nonce, plaintext.encode(), None)
    
    # Format: base64(nonce + ciphertext)
    combined = nonce + ciphertext
    return base64.b64encode(combined).decode()

def decrypt_secret(ciphertext_b64: str, key_hex: str) -> str:
    if not key_hex:
        return ciphertext_b64
    
    # Convert hex key to bytes
    key = bytes.fromhex(key_hex)
    if len(key) != 32:
        raise ValueError("Encryption key must be 32 bytes (64 hex chars)")
    
    # Decode base64
    combined = base64.b64decode(ciphertext_b64)
    
    # Extract nonce and ciphertext
    nonce = combined[:12]
    ciphertext = combined[12:]
    
    # Decrypt
    aesgcm = AESGCM(key)
    plaintext = aesgcm.decrypt(nonce, ciphertext, None)
    return plaintext.decode()

def main():
    if len(sys.argv) != 4:
        print("Usage: encrypt_secret.py {encrypt|decrypt} <data> <key_hex>", file=sys.stderr)
        sys.exit(1)
    
    action = sys.argv[1]
    data = sys.argv[2]
    key_hex = sys.argv[3]
    
    try:
        if action == "encrypt":
            result = encrypt_secret(data, key_hex)
            print(result)
        elif action == "decrypt":
            result = decrypt_secret(data, key_hex)
            print(result)
        else:
            print("Unknown action: {encrypt|decrypt}", file=sys.stderr)
            sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
