# twoWiki — Lovable style pack (`lovable_01a`)

Fossil skin **package** (the usual five files). **`apply_twowiki_skin.py`** in the parent directory reads these into repository `config`:

| File | `config.name` |
|------|----------------|
| `css.txt` | `css` (concatenated with `../twowiki-fossil-th1-append.css` for tickets, Mermaid overflow, float resets) |
| `header.txt` | `header` |
| `details.txt` | `details` |
| `js.txt` | `js` |

**Not used from this folder:** `footer.txt` — production deploy keeps **`../footer.th1`** so Mermaid/ELK, ticket redirect, and Setup links stay intact.

Edit **`css.txt`** / **`header.txt`** here for visual changes; extend **`../twowiki-fossil-th1-append.css`** for behavior that must ride after package CSS.
