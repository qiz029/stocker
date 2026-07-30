ALTER TABLE room_players
    ADD COLUMN debt_cents BIGINT NOT NULL DEFAULT 0,
    -- interest_through_day: last sim day interest was accrued through
    -- (NULL = never borrowed). Borrowing on day d starts accruing on d+1.
    ADD COLUMN interest_through_day INT,
    -- bankrupt_day: sim day the debt cap was crossed (NULL = active).
    ADD COLUMN bankrupt_day INT;

-- The market proxy instrument id for the loan interest rate (vol20 of its
-- log closes); '' falls back to an equal-weighted basket of the scenario's
-- instruments. Display-neutral: the engine never reads this.
ALTER TABLE scenarios ADD COLUMN market_proxy TEXT NOT NULL DEFAULT '';

CREATE TABLE loan_txns (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    day INT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('borrow', 'repay')),
    amount_cents BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX loan_txns_room ON loan_txns (room_id, user_id, id);

-- Per-player, per-day net-asset snapshot written by settlement; feeds the
-- leaderboard equity curve.
CREATE TABLE room_player_daily (
    room_id BIGINT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    day INT NOT NULL,
    total_cents BIGINT NOT NULL,
    PRIMARY KEY (room_id, user_id, day)
);
