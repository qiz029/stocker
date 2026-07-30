-- Rolling option chain: long-only European calls/puts on room instruments,
-- cash-settled automatically at expiry and priced by Black-Scholes on
-- realized volatility. Expiries land every 5 sim days; strikes anchor to the
-- listing day's close (80/90/100/110/120%), and the UNIQUE key makes the
-- rolling listing idempotent (ON CONFLICT DO NOTHING).
CREATE TABLE room_options (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    instrument_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('call', 'put')),
    strike DOUBLE PRECISION NOT NULL,
    expiry_day INT NOT NULL,
    listed_day INT NOT NULL,
    UNIQUE (room_id, instrument_id, kind, strike, expiry_day)
);
CREATE INDEX room_options_room ON room_options (room_id, instrument_id, expiry_day);

-- Long positions only; premium_paid (dollars) is the remaining average-method
-- cost basis, reduced proportionally on sell-to-close.
CREATE TABLE option_positions (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    option_id BIGINT NOT NULL REFERENCES room_options(id) ON DELETE CASCADE,
    contracts DOUBLE PRECISION NOT NULL,
    premium_paid DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (room_id, user_id, option_id)
);

-- Every fill: buys, sell-to-close, and automatic expiry settlement
-- (amount_cents 0 when the contract expires worthless).
CREATE TABLE option_trades (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    option_id BIGINT NOT NULL REFERENCES room_options(id) ON DELETE CASCADE,
    day INT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('buy', 'sell', 'expiry')),
    contracts DOUBLE PRECISION NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    amount_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX option_trades_room ON option_trades (room_id, user_id, id);
