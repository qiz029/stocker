CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE scenarios (
    id TEXT PRIMARY KEY,
    days INT NOT NULL,
    factors JSONB NOT NULL,
    key_windows JSONB NOT NULL
);

-- ord preserves declaration order; loading must not sort by id
-- ("S10" < "S2" lexicographically would silently reorder betas/state).
CREATE TABLE instruments (
    scenario_id TEXT NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    ord INT NOT NULL,
    alias TEXT NOT NULL,
    descr TEXT NOT NULL DEFAULT '',
    real_name TEXT NOT NULL DEFAULT '',
    beta JSONB NOT NULL,
    PRIMARY KEY (scenario_id, id)
);

CREATE TABLE scenario_prices (
    scenario_id TEXT NOT NULL,
    instrument_id TEXT NOT NULL,
    day INT NOT NULL,
    open DOUBLE PRECISION NOT NULL,
    high DOUBLE PRECISION NOT NULL,
    low DOUBLE PRECISION NOT NULL,
    close DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (scenario_id, instrument_id, day),
    FOREIGN KEY (scenario_id, instrument_id)
        REFERENCES instruments (scenario_id, id) ON DELETE CASCADE
);

CREATE TABLE rooms (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invite_code TEXT NOT NULL UNIQUE,
    scenario_id TEXT NOT NULL REFERENCES scenarios(id),
    days INT NOT NULL,
    seed BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'lobby' CHECK (status IN ('lobby', 'running')),
    day_duration_secs INT NOT NULL,
    started_at TIMESTAMPTZ,
    host_user_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE room_players (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    cash_cents BIGINT NOT NULL,
    joined_day INT NOT NULL DEFAULT 0,
    PRIMARY KEY (room_id, user_id)
);

CREATE TABLE room_prices (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    instrument_id TEXT NOT NULL,
    day INT NOT NULL,
    open DOUBLE PRECISION NOT NULL,
    high DOUBLE PRECISION NOT NULL,
    low DOUBLE PRECISION NOT NULL,
    close DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (room_id, instrument_id, day)
);

-- track / true_shock / report_shock are server-side only (blind box).
CREATE TABLE room_news (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    day INT NOT NULL,
    media_id TEXT NOT NULL,
    headline TEXT NOT NULL,
    track TEXT NOT NULL,
    true_shock JSONB,
    report_shock JSONB
);
CREATE INDEX room_news_room_day ON room_news (room_id, day, id);

CREATE TABLE orders (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    instrument_id TEXT NOT NULL,
    side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    amount_cents BIGINT NOT NULL DEFAULT 0,
    shares DOUBLE PRECISION NOT NULL DEFAULT 0,
    exec_day INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'filled', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX orders_pending ON orders (room_id, exec_day) WHERE status = 'pending';

CREATE TABLE trades (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES orders(id),
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    instrument_id TEXT NOT NULL,
    side TEXT NOT NULL,
    day INT NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    shares DOUBLE PRECISION NOT NULL,
    amount_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX trades_room ON trades (room_id, day, id);

CREATE TABLE positions (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    instrument_id TEXT NOT NULL,
    shares DOUBLE PRECISION NOT NULL,
    PRIMARY KEY (room_id, user_id, instrument_id)
);

CREATE TABLE room_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    day INT NOT NULL,
    kind TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX room_events_room ON room_events (room_id, id);
