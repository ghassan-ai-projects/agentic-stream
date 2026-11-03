-- Rule promotion (AS-6): whether a rule's match fell inside its approved cap.
-- An active rule that matches outside its envelope is recorded beside the
-- model's episode like a shadow rule; when most of its recent matches fall
-- outside the cap, its circuit opens (out_of_cap).
ALTER TABLE rule_evaluations ADD COLUMN in_cap INTEGER NOT NULL DEFAULT 1 CHECK (in_cap IN (0, 1));
