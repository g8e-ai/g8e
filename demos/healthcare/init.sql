CREATE TABLE pa_requests (
    id                VARCHAR(20) PRIMARY KEY,
    request_type      VARCHAR(20) NOT NULL,   -- STANDARD | EXPEDITED
    status            VARCHAR(30) NOT NULL,
    days_elapsed      INTEGER     NOT NULL DEFAULT 0,
    reportable_to_oha BOOLEAN     NOT NULL DEFAULT FALSE
);

-- Standard PAs: 12 total, 3 DENIED = 25.00% denial rate
INSERT INTO pa_requests VALUES
  ('PA-2026-0001', 'STANDARD', 'APPROVED',        3, false),
  ('PA-2026-0002', 'STANDARD', 'APPROVED',        5, false),
  ('PA-2026-0003', 'STANDARD', 'APPROVED',        2, false),
  ('PA-2026-0004', 'STANDARD', 'APPROVED',        6, false),
  ('PA-2026-0005', 'STANDARD', 'APPROVED',        4, false),
  ('PA-2026-0006', 'STANDARD', 'APPROVED',        1, false),
  ('PA-2026-0007', 'STANDARD', 'APPROVED',        7, false),
  ('PA-2026-0008', 'STANDARD', 'DENIED',          3, false),
  ('PA-2026-0009', 'STANDARD', 'DENIED',          5, false),
  ('PA-2026-0010', 'STANDARD', 'DENIED',          4, false),
  -- documented demo cases (matches pa_requests.json IDs)
  ('PA-2026-0041', 'STANDARD', 'PENDING_REVIEW',  6, false),
  ('PA-2026-0042', 'STANDARD', 'IN_REVIEW',       2, false),
-- Expedited PAs: 8 total, 3 DENIED = 37.50% denial rate
  ('PA-2026-0043', 'EXPEDITED', 'AUTO_APPROVED',  0, false),
  ('PA-2026-0044', 'EXPEDITED', 'SLA_BREACHED',  10, true),
  ('PA-2026-0045', 'EXPEDITED', 'APPROVED',       1, false),
  ('PA-2026-0046', 'EXPEDITED', 'APPROVED',       2, false),
  ('PA-2026-0047', 'EXPEDITED', 'APPROVED',       3, false),
  ('PA-2026-0048', 'EXPEDITED', 'DENIED',         2, false),
  ('PA-2026-0049', 'EXPEDITED', 'DENIED',         4, false),
  ('PA-2026-0050', 'EXPEDITED', 'DENIED',         1, false);
