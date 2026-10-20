-- A command binding records the target the governed command named (its
-- normalized target) when the effector bound it to a different device output,
-- so reconciliation evidence presented for the command is compared like with
-- like. NULL keeps the earlier rule: the command named the bound output itself.
ALTER TABLE device_command_bindings ADD COLUMN normalized_target TEXT;
