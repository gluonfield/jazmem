package memfs

import "strings"

func LongTermDreamGuidance() string {
	return strings.TrimSpace(`LONG_TERM.md is profile-level memory, not a changelog and not project implementation memory.

Include only:
- identity, biography, location/life context, and major past work that shape future interactions;
- major goals, north stars, constraints, and values that should matter across unrelated sessions;
- deep standing preferences that reflect how the user wants to work broadly, not routine repo-level style;
- genuinely key relationships and organization contexts: close collaborators, friends, major professional contexts, investors/advisors/customers/partners, or organizations central to the user's trajectory.

Promote a fact to LONG_TERM.md only if it should still matter months from now across unrelated sessions, or if the user directly states it as part of who they are, what they want, or a key relationship.

Do not promote:
- routine coding style preferences, repo-specific engineering rules, feature decisions, bug reports, UI choices, or implementation notes;
- comments about Jaz/jazmem internals unless they reveal a stable personal operating principle;
- every person/company page, weak connection, or one-off meeting. A person the user met once belongs on a canonical page or daily note, not under Key Relationships in LONG_TERM.md.

If an existing LONG_TERM.md item fails this bar, remove it from LONG_TERM.md only after ensuring the fact survives in a canonical page, daily page, or review note.`)
}

func ShortTermDreamGuidance() string {
	return strings.TrimSpace(`SHORT_TERM.md is the present working set, not the user's profile and not a historical archive.

Keep current focus, active projects, active blockers, active open loops, and near-term decisions. Feature-specific preferences and implementation constraints can live here while the work is active, then should be pruned or moved to canonical project/concept pages when stale.

Do not use SHORT_TERM.md as a substitute for canonical pages or daily history. Replace stale lines in place; daily pages preserve what happened.`)
}
