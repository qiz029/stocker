-- Agent messages carry a translated copy so the chat follows the room UI
-- language without translating player-authored messages.
ALTER TABLE room_chat ADD COLUMN text_en TEXT NOT NULL DEFAULT '';
