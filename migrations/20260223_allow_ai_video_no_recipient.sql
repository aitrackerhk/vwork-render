-- Allow ai_video type messages without recipient (to_user_id and to_customer_id can both be NULL)
-- This fixes the 500 error when creating video projects via VaiVideo.createProject()

-- Drop existing constraint
ALTER TABLE messages DROP CONSTRAINT IF EXISTS chk_message_recipient;

-- Re-add constraint: allow ai_chat, public_chat, and ai_video without recipient
ALTER TABLE messages ADD CONSTRAINT chk_message_recipient 
    CHECK (
        (message_type = 'ai_chat') OR 
        (message_type = 'public_chat') OR 
        (message_type = 'ai_video') OR 
        (to_user_id IS NOT NULL) OR 
        (to_customer_id IS NOT NULL)
    );
