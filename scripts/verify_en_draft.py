#!/usr/bin/env python3
"""
verify_en_draft.py
Memverifikasi konsistensi draft/en/naskah-artikel-en.md terhadap naskah asli draft/naskah-artikel.md.
"""
import re
import sys

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')

with open('draft/naskah-artikel.md', 'r', encoding='utf-8') as f:
    id_text = f.read()

with open('draft/en/naskah-artikel-en.md', 'r', encoding='utf-8') as f:
    en_text = f.read()

# Potong teks sebelum daftar pustaka untuk audit sitasi dalam teks
id_body = id_text.split('## Daftar Pustaka')[0]
en_body = en_text.split('## References')[0]

print("=" * 60)
print("AUDIT KONSISTENSI NASKAH ASLI (ID) VS DRAFT BAHASA INGGRIS (EN)")
print("Cakupan: Seluruh Naskah (Title, Abstract, Bab 1-5, Declarations, References)")
print("=" * 60)

# 1. Audit Tanda Baca Em-Dash & En-Dash
em_dashes_en = len(re.findall(r'\u2014', en_text))
en_dashes_en = len(re.findall(r'\u2013', en_text))
print(f"[TANDA BACA] Em-Dash (\\u2014): {em_dashes_en} (Target: 0)")
print(f"[TANDA BACA] En-Dash (–): {en_dashes_en} (Rentang numerik/halaman)")

# 2. Audit Double Space & Trailing Space
lines_en = en_text.splitlines()
double_spaces = [i for i, l in enumerate(lines_en, 1) if not l.startswith('|') and re.search(r'[^\s] {2,}[^\s]', l)]
trailing_spaces = [i for i, l in enumerate(lines_en, 1) if l.endswith(' ')]
print(f"[SPASI] Double spaces: {len(double_spaces)} (Baris: {double_spaces})")
print(f"[SPASI] Trailing spaces: {len(trailing_spaces)} (Baris: {trailing_spaces})")

# 3. Audit Sitasi (APA 7th dalam naskah)
NON_CITATIONS = {'ZTA', 'Naif', 'PBS', 'Cache', 'Baseline', 'Table', 'Tabel', 'Figure', 'Gambar', 'Pada', 'Ed', 'NIST', 'SP', 'IEEE', 'ACM', 'Under'}
raw_id_cits = set(re.findall(r'\b([A-Z][a-zA-Z\u00C0-\u017F]+(?:\s+et\s+al\.)?)\s*\(?(\d{4})\)?', id_body))
raw_en_cits = set(re.findall(r'\b([A-Z][a-zA-Z\u00C0-\u017F]+(?:\s+et\s+al\.)?)\s*\(?(\d{4})\)?', en_body))

id_cits = {c for c in raw_id_cits if c[0] not in NON_CITATIONS}
en_cits = {c for c in raw_en_cits if c[0] not in NON_CITATIONS}

missing_cits = id_cits - en_cits
extra_cits = en_cits - id_cits
print(f"\n[SITASI DALAM BODY TEKS] Total Sitasi ID: {len(id_cits)}, Total Sitasi EN: {len(en_cits)}")
print(f"[SITASI] Missing in EN: {missing_cits}")
print(f"[SITASI] Extra in EN: {extra_cits}")

# 3b. Audit Daftar Referensi (Bibliography)
id_refs = [r.strip() for r in id_text.split('## Daftar Pustaka')[1].strip().split('\n\n') if r.strip()]
en_refs = [r.strip() for r in en_text.split('## References')[1].strip().split('\n\n') if r.strip()]
print(f"[DAFTAR PUSTAKA] ID Entries: {len(id_refs)}, EN Entries: {len(en_refs)}, Match: {len(id_refs) == len(en_refs)}")

# 4. Audit Nilai Numerik & Statistik Kunci (Bab 1-4)
key_metrics = [
    ("U 500", "503.0"),
    ("p 500", "0.438"),
    ("Cliff delta 500", "+0.12"),
    ("TOST", "0.210"),
    ("500 RPS", "500"),
    ("2000 RPS", "2000"),
    ("5000 RPS", "5000"),
    ("p99 @ 2000", "2243.83"),
    ("p99 @ 5000", "12898.73"),
    ("p99 S07 Fan-out", "6140.55"),
    ("p99 S08 Mesh", "4028.93"),
    ("p99 S10 Static", "5927.94"),
    ("U static S10 vs S07", "436.0"),
    ("p static S10 vs S07", "0.842"),
    ("revocation p99", "183.39"),
    ("revocation median", "18.65 ms"),
    ("Kruskal-Wallis H Topology", "63.40"),
    ("CPU KW H", "2.79"),
    ("Memory KW H", "0.91"),
    ("Tbase", "30,000"),
    ("Clamped lower", "5,000"),
    ("Weight w", "0.7"),
    ("Kendall tau", "1.0"),
    ("Node vCPU", "2 vCPU"),
    ("Node RAM", "4 GB RAM"),
    ("Bootstrap B", "10,000"),
    ("Random seed", "42"),
    ("TOST margin", "14.79"),
    ("Memory S03", "257.62"),
    ("Local cache quota", "K_{\\text{local}}")
]

print("\n[METRIK EMPIRIS]")
all_metrics_ok = True
for label, val in key_metrics:
    found = val in en_text
    status = "PASS" if found else "FAIL"
    if not found:
        all_metrics_ok = False
    print(f"  [{status}] {label}: '{val}'")

# 5. Audit Persamaan Matematika (Equations 1-4)
print("\n[PERSAMAAN MATEMATIKA (1-4)]")
equations_ok = True
for eq_num in range(1, 5):
    tag = f"...({eq_num})"
    found = tag in en_text
    status = "PASS" if found else "FAIL"
    if not found:
        equations_ok = False
    print(f"  [{status}] Equation ({eq_num}): '{tag}'")

# 6. Audit Gambar (Figures 1-10)
print("\n[GAMBAR & FIGUR (1-10)]")
figures_ok = True
for fig_num in range(1, 11):
    tag = f"Figure {fig_num}."
    found = tag in en_text
    status = "PASS" if found else "FAIL"
    if not found:
        figures_ok = False
    print(f"  [{status}] Figure {fig_num}: '{tag}'")

# 7. Audit Tabel (Tables 1-12)
print("\n[TABEL UTAMA (1-12)]")
tables_ok = True
table_tags = ["Table 1.", "Table 2.", "Table 3.", "Table 4.", "Table 5.", "Table 6.", "Table 7.", "Table 8.", "Table 9.", "Table 10.", "Table 11.", "Table 12."]
for tag in table_tags:
    found = tag in en_text
    status = "PASS" if found else "FAIL"
    if not found:
        tables_ok = False
    print(f"  [{status}] {tag}")

# 8. Audit Struktur Markdown Tabel
table_lines = [l for l in lines_en if l.startswith('|')]
print(f"\n[INTEGRITAS TABEL MARKDOWN] Total baris tabel: {len(table_lines)}")

fn_defs = len(re.findall(r'^\[\^\d+\]:', en_text, re.MULTILINE))
fn_inline = len(re.findall(r'\[\^\d+\]', en_text)) - fn_defs
fn_balanced = (fn_inline == fn_defs and fn_defs > 0)
print(f"[FOOTNOTE] Refs: {fn_inline}, Defs: {fn_defs}, Seimbang: {fn_balanced}")

math_pairs = len(re.findall(r'\$[^\$]+\$', en_text))
print(f"[MATH INLINE] Formula tertutup sempurna: {math_pairs} pasang")

print("=" * 60)
if em_dashes_en == 0 and len(double_spaces) == 0 and len(missing_cits) == 0 and all_metrics_ok and equations_ok and figures_ok and tables_ok and fn_balanced:
    print("STATUS KONSISTENSI: 100% SESUAI DENGAN NASKAH ACUAN")
else:
    print("STATUS KONSISTENSI: ADA CATATAN YANG PERLU DIPERIKSA")
print("=" * 60)
