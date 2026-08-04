-- Give the five built-in competitors stable fictional investing personas.
-- They remain explicitly marked as agents in every public API/UI surface.
ALTER TABLE users ADD COLUMN agent_name_en TEXT;

UPDATE users SET
    agent_name = CASE agent_slot
        WHEN 1 THEN '西雅图价值客'
        WHEN 2 THEN '硅谷链上哥'
        WHEN 3 THEN '奥马哈复利派'
        WHEN 4 THEN '华尔街逆向姐'
        WHEN 5 THEN '湾区趋势侠'
    END,
    agent_name_en = CASE agent_slot
        WHEN 1 THEN 'Seattle Value Sage'
        WHEN 2 THEN 'Silicon Valley Chain Bull'
        WHEN 3 THEN 'Omaha Compounder'
        WHEN 4 THEN 'Wall Street Contrarian'
        WHEN 5 THEN 'Bay Area Momentum'
    END
WHERE is_agent;

ALTER TABLE room_forum_posts ADD COLUMN is_agent BOOLEAN NOT NULL DEFAULT FALSE;
