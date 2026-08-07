-- +goose Up

-- Where a character is standing stops being a district (#12).
--
-- §3.12's layers are home grid → interior → district → city map, and until now
-- a character could only ever be in the third of them: the column was called
-- `district` and held one of four keys. With portals and the location graph a
-- player stands in their own kitchen, in the corner shop, or on the street, and
-- all three are places in internal/city rather than three different kinds of
-- thing.
--
-- So the column is renamed rather than added to. There is no second column for
-- "which interior" — that would make "where are you" a question with two
-- answers and a rule about which one wins, and the first bug would be a player
-- standing in a shop in a district they had walked out of.
ALTER TABLE character RENAME COLUMN district TO place;

COMMENT ON COLUMN character.place IS
    'Where they are standing: any place key from internal/city, on any layer.';
COMMENT ON COLUMN character.home IS
    'Where they live: the home-layer place they walk back to (§3.12).';

-- Home becomes the flat rather than the neighbourhood.
--
-- Every character made before this migration lives in 'east-side', which was
-- the only answer the vocabulary had. The home layer now exists and there is
-- exactly one home in it, so "where you live" can finally be a door rather than
-- a postcode. Anything else is left alone: a key this build does not recognise
-- keeps its key rather than being rewritten (internal/character.placeOrKey), and
-- guessing on a player's behalf is how a save gets quietly edited.
UPDATE character SET home = 'east-side-apartment', updated_at = now()
 WHERE home = 'east-side';
