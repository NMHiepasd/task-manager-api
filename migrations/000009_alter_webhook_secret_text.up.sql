-- Migration 000009: Alter webhook secret column from VARCHAR(255) to TEXT to accommodate Base64-encoded AES-256-GCM ciphertexts
ALTER TABLE webhooks ALTER COLUMN secret TYPE TEXT;
