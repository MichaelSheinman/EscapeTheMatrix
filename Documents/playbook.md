A player's playbook define how the player's avatar behaves day-to-day. 

A key concept of the game is that the player does not execute day-to-day actions. Instead, a big game mechanic is that the player often reviews past events, and updates their playbook periodically.

Ultimately, none of the playbook actions are "correct", and that's the trick. There is only an effective playbook set for a given environment. But environment also change overtime, so a player needs to keep adopting the playbook.  


## Logical Builder 
Develop a logical builder, a user can set a combination of up to 2 conditions within the builder, joined by a single `and` or `or`:
A
A and B
A or B

No nesting in V1. Three conditions with nesting — `(A and B) or C` — is deferred. The ceiling is a design choice, not a technical limit: two terms is as much as a rule can carry and still read as a sentence.


Questions the playbook meant to answer?


1. When should a character say yes if someone asks them for help with a task? Say yes if (options)

---> I am being paid above $X for this help. (works well in orange environment, but very poorly in green, and bad in red too since people don't pay) 
---> I think this person can help me in the future. (works well in Orange environment, but not in red as people often lie) 
---> The person asking for help has a Karma above X (only works well after high karma values, undertermenstic at low karma values) 
---> Completing the task takes less than X hours (does not work well in orange environments, which require long hours)
---> The task is considered legal to do. (works well in green/orange environments, but not red ones)
---> This person is a potential future friend. (works well in green environments, not orange/red, people will abuse you) 


2. What should the character do when someone raises their voice at you?
---> Raise voice back (Works well in Red, Orange, but poorly in green)
---> Ask them how they feel (works well in green, poor in red/orange)
---> Leave the situation (works well in orange)
---> Be quiet and don't speak up (Works well in blue)

3. When should a character do when someone ask them for money? 
---> Don't give money (good for orange, red)
---> Give them up to X money (good for green societies ++ karma)
---> Ask them to give you interest back on it (good for orange, bad strategy in red unless you are really strong)

4. When should a character do when asked to work overtime? 
---> Accept it 
---> Accept it if it pays above X my normal salary (good for orange)
---> Never work overtime (bad for orange societies)


5. How should the character make friends? 
---> Only make friends with people above $X money
---> Make friends with anyone 
---> Make friends with anyone you feel connected with 
---> Make friends with whoever looks most powerful 
