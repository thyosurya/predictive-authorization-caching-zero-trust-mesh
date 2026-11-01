#!/usr/bin/env python3
"""
scripts/export_to_latex.py
Converts draft/naskah-artikel.md and draft/en/naskah-artikel-en.md into
publication-ready LaTeX (.tex) manuscripts (IEEEtran / academic format)
and generates draft/references.bib.
"""

import re
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent


def escape_latex_text(text: str) -> str:
    """Escape LaTeX special characters in non-math, non-code text."""
    # Replace special characters
    text = text.replace("\\", r"\textbackslash{}")
    text = text.replace("&", r"\&")
    text = text.replace("%", r"\%")
    text = text.replace("$", r"\$")
    text = text.replace("#", r"\#")
    text = text.replace("_", r"\_")
    text = text.replace("{", r"\{")
    text = text.replace("}", r"\}")
    text = text.replace("~", r"\textasciitilde{}")
    text = text.replace("^", r"\textasciicircum{}")
    # Unicode math/typography replacements in text mode
    text = text.replace("≤", r"$\le$")
    text = text.replace("≥", r"$\ge$")
    text = text.replace("×", r"$\times$")
    text = text.replace("→", r"$\rightarrow$")
    text = text.replace("↔", r"$\leftrightarrow$")
    text = text.replace("≈", r"$\approx$")
    text = text.replace("–", "--")
    return text


def format_inline_markdown(line: str) -> str:
    """
    Convert inline Markdown ($math$, `code`, **bold**, *italic*, URLs) to LaTeX
    while protecting math and code segments from escaping.
    """
    tokens = []

    def save_token(val: str) -> str:
        idx = len(tokens)
        tokens.append(val)
        return f"@@TOK{idx}@@"

    # 1. Protect inline math $...$ (keep full math notation, allow wrap at ';\ ' and ',\ ')
    def repl_math(m):
        raw = m.group(0)
        raw = raw.replace(r";\ ", r";\allowbreak\ ")
        raw = raw.replace(r",\ ", r",\allowbreak\ ")
        return save_token(raw)

    line = re.sub(r"\$(?!\$)(.+?)\$", repl_math, line)

    # 2. Protect inline code `...` (allow line-breaking inside long monospace strings)
    def repl_code(m):
        inner = escape_latex_text(m.group(1))
        inner = inner.replace(r"\_", r"\_\allowbreak{}")
        inner = inner.replace("/", r"/\allowbreak{}")
        inner = inner.replace("(", r"(\allowbreak{}")
        inner = inner.replace(r"\{", r"\{\allowbreak{}")
        inner = inner.replace("=", r"=\allowbreak{}")
        inner = inner.replace(":", r":\allowbreak{}")
        inner = inner.replace(",", r",\allowbreak{}")
        inner = inner.replace("-", r"-\allowbreak{}")
        return save_token(f"\\texttt{{{inner}}}")

    line = re.sub(r"`([^`]+)`", repl_code, line)

    # 3. Protect URLs
    def repl_url(m):
        url = m.group(1)
        trailing = ""
        while url and url[-1] in ".,;)":
            trailing = url[-1] + trailing
            url = url[:-1]
        return save_token(f"\\url{{{url}}}") + trailing

    line = re.sub(r"(https?://[^\s<>]+)", repl_url, line)

    # 4. Escape remaining plain text (preserving *, [, ], ^1 markers temporarily)
    # Handle footnote ref [^1]
    line = line.replace("[^1]", "@@FOOTNOTE1@@")

    # Escape plain text segments outside tokens
    parts = re.split(r"(@@TOK\d+@@|@@FOOTNOTE1@@)", line)
    for i in range(len(parts)):
        if not (parts[i].startswith("@@TOK") or parts[i] == "@@FOOTNOTE1@@"):
            parts[i] = escape_latex_text(parts[i])
    line = "".join(parts)

    # 5. Convert **bold** and *italic*
    line = re.sub(r"\*\*(.+?)\*\*", r"\\textbf{\1}", line)
    line = re.sub(r"\*(.+?)\*", r"\\textit{\1}", line)

    # 6. Restore tokens
    for idx in range(len(tokens) - 1, -1, -1):
        line = line.replace(f"@@TOK{idx}@@", tokens[idx])

    return line


def convert_display_math(math_block: str) -> str:
    """Convert $$...$$ into numbered equation or \\[ ... \\]."""
    inner = math_block.strip()
    if inner.startswith("$$") and inner.endswith("$$"):
        inner = inner[2:-2].strip()

    # Check if numbered equation \quad \text{...(N)}
    m = re.search(r"\\quad\s*\\text\{\.\.\.\((\d+)\)\}", inner)
    if m:
        eq_num = m.group(1)
        inner = re.sub(r"\\quad\s*\\text\{\.\.\.\(\d+\)\}", "", inner).strip()
        return f"\\begin{{equation}}\\label{{eq:eq{eq_num}}}\n{inner}\n\\end{{equation}}"
    else:
        return f"\\[\n{inner}\n\\]"


def convert_markdown_to_latex(md_path: Path, is_english: bool = False) -> str:
    raw_text = md_path.read_text(encoding="utf-8")
    lines = raw_text.splitlines()

    # Extract title, author, footnote, Abstrak, Abstract
    title = "Predictive Authorization Caching with Microservice Graph Prioritization in Zero-Trust Service Mesh"
    author_footnote = (
        "Department of Information Systems, Universitas Negeri Gorontalo, Indonesia (e-mail: thyo.surya@gmail.com)."
        if is_english
        else "Program Studi Sistem Informasi, Universitas Negeri Gorontalo, Indonesia (e-mail: thyo.surya@gmail.com)."
    )

    body_out = []
    in_references = False
    ref_items = []

    i = 0
    n = len(lines)

    # Find start of Section 1 and extract abstracts
    abstrak_id = ""
    keywords_id = ""
    abstract_en = ""
    keywords_en = ""

    sec1_idx = 0
    for idx, l in enumerate(lines):
        if l.startswith("## 1."):
            sec1_idx = idx
            break

    header_block = "\n".join(lines[:sec1_idx])
    if is_english:
        m_abs = re.search(r"## Abstract\s+(.+?)\s+\*\*Keywords:\*\*\s*(.+?)(?:\n---|\Z)", header_block, re.S)
        if m_abs:
            abstract_en = format_inline_markdown(m_abs.group(1).strip())
            keywords_en = format_inline_markdown(m_abs.group(2).strip())
    else:
        m_abs_id = re.search(r"## Abstrak\s+(.+?)\s+\*\*Kata Kunci:\*\*\s*(.+?)\n---", header_block, re.S)
        if m_abs_id:
            abstrak_id = format_inline_markdown(m_abs_id.group(1).strip())
            keywords_id = format_inline_markdown(m_abs_id.group(2).strip())
        m_abs_en = re.search(r"## Abstract\s+(.+?)\s+\*\*Keywords:\*\*\s*(.+?)\n---", header_block, re.S)
        if m_abs_en:
            abstract_en = format_inline_markdown(m_abs_en.group(1).strip())
            keywords_en = format_inline_markdown(m_abs_en.group(2).strip())

    i = sec1_idx
    pending_caption = None
    pending_label = None
    pending_kind = None  # "table" or "figure"

    while i < n:
        line = lines[i].strip()

        # Skip horizontal rules or empty lines
        if not line or line == "---":
            i += 1
            continue

        # Multi-line or single-line display math $$...$$
        if line.startswith("$$"):
            math_lines = [ lines[i] ]
            if not (line.endswith("$$") and len(line) > 4):
                i += 1
                while i < n:
                    math_lines.append(lines[i])
                    if lines[i].strip().endswith("$$"):
                        break
                    i += 1
            body_out.append(convert_display_math("\n".join(math_lines)))
            body_out.append("")
            i += 1
            continue

        # Section headings
        if line.startswith("## "):
            sec_title = line[3:].strip()
            if sec_title in ("Daftar Pustaka", "References"):
                in_references = True
                i += 1
                continue
            # Strip leading number like "1. Pendahuluan" -> "Pendahuluan"
            sec_clean = re.sub(r"^\d+\.\s*", "", sec_title)
            if sec_clean in ("Pernyataan", "Declarations"):
                body_out.append(f"\\section*{{{format_inline_markdown(sec_clean)}}}")
            else:
                body_out.append(f"\\section{{{format_inline_markdown(sec_clean)}}}")
            body_out.append("")
            i += 1
            continue

        if line.startswith("### "):
            sub_title = line[4:].strip()
            sub_clean = re.sub(r"^\d+\.\d+\s*", "", sub_title)
            body_out.append(f"\\subsection{{{format_inline_markdown(sub_clean)}}}")
            body_out.append("")
            i += 1
            continue

        # If in References section
        if in_references:
            ref_items.append(format_inline_markdown(line))
            i += 1
            continue

        # Detect Table caption: **Tabel X.** Title or **Table X.** Title
        m_tab = re.match(r"^\*\*(?:Tabel|Table)\s+(\d+)\.\*\*\s*(.+)$", line)
        if m_tab:
            pending_kind = "table"
            pending_label = f"tab:table{m_tab.group(1)}"
            pending_caption = format_inline_markdown(m_tab.group(2).strip())
            i += 1
            continue

        # Detect Figure caption: **Gambar X.** Title or **Figure X.** Title
        m_fig = re.match(r"^\*\*(?:Gambar|Figure)\s+(\d+)\.\*\*\s*(.+)$", line)
        if m_fig:
            pending_kind = "figure"
            pending_label = f"fig:fig{m_fig.group(1)}"
            pending_caption = format_inline_markdown(m_fig.group(2).strip())
            i += 1
            continue

        # Detect Image ![alt](path)
        m_img = re.match(r"^!\[(.*?)\]\((.+?)\)$", line)
        if m_img:
            img_path = m_img.group(2).strip()
            cap = pending_caption if (pending_kind == "figure" and pending_caption) else format_inline_markdown(m_img.group(1))
            lbl = pending_label if (pending_kind == "figure" and pending_label) else "fig:auto"
            body_out.append("\\begin{figure}[htbp]")
            body_out.append("\\centering")
            body_out.append(f"\\includegraphics[width=0.85\\textwidth]{{{img_path}}}")
            body_out.append(f"\\caption{{{cap}}}")
            body_out.append(f"\\label{{{lbl}}}")
            body_out.append("\\end{figure}")
            body_out.append("")
            pending_kind = None
            pending_caption = None
            pending_label = None
            i += 1
            continue

        # Detect Markdown Table
        if line.startswith("|"):
            table_lines = []
            while i < n and lines[i].strip().startswith("|"):
                table_lines.append(lines[i].strip())
                i += 1

            # Check if followed by a note line (*Catatan:...* or *Note:...* or Catatan:...)
            note_line = None
            temp_i = i
            while temp_i < n and not lines[temp_i].strip():
                temp_i += 1
            if temp_i < n:
                cand = lines[temp_i].strip()
                if cand.startswith("*Catatan:") or cand.startswith("*Note:") or cand.startswith("Catatan:") or cand.startswith("Note:"):
                    note_line = format_inline_markdown(cand)
                    i = temp_i + 1

            # Parse rows
            rows = []
            for tl in table_lines:
                cells = [c.strip() for c in tl.strip("|").split("|")]
                # Skip separator row |---|---|
                if all(re.match(r"^:?-{2,}:?$", c) for c in cells):
                    continue
                rows.append([format_inline_markdown(c) for c in cells])

            if rows:
                num_cols = len(rows[0])
                col_spec = "l" + "c" * (num_cols - 1) if num_cols > 2 else "ll"
                cap = pending_caption if (pending_kind == "table" and pending_caption) else "Table"
                lbl = pending_label if (pending_kind == "table" and pending_label) else "tab:auto"

                body_out.append("\\begin{table}[htbp]")
                body_out.append("\\centering")
                body_out.append(f"\\caption{{{cap}}}")
                body_out.append(f"\\label{{{lbl}}}")
                body_out.append("\\fittable{%")
                body_out.append(f"\\begin{{tabular}}{{{col_spec}}}")
                body_out.append("\\toprule")
                header_cells = [f"\\textbf{{{c}}}" if not c.startswith("\\textbf") else c for c in rows[0]]
                body_out.append(" & ".join(header_cells) + " \\\\")
                body_out.append("\\midrule")
                for r in rows[1:]:
                    # Pad or trim if needed
                    while len(r) < num_cols:
                        r.append("")
                    body_out.append(" & ".join(r[:num_cols]) + " \\\\")
                body_out.append("\\bottomrule")
                body_out.append("\\end{tabular}%")
                body_out.append("}")
                if note_line:
                    body_out.append(f"\\par\\vspace{{4pt}}\\footnotesize {note_line}")
                body_out.append("\\end{table}")
                body_out.append("")

            pending_kind = None
            pending_caption = None
            pending_label = None
            continue

        # Detect unordered list (- item or * item)
        if line.startswith("- ") or line.startswith("* "):
            body_out.append("\\begin{itemize}")
            while i < n:
                sline = lines[i].strip()
                if sline.startswith("- ") or sline.startswith("* "):
                    item_txt = sline[2:].strip()
                    body_out.append(f"  \\item {format_inline_markdown(item_txt)}")
                    i += 1
                elif not sline and i + 1 < n and (lines[i + 1].strip().startswith("- ") or lines[i + 1].strip().startswith("* ")):
                    i += 1
                else:
                    break
            body_out.append("\\end{itemize}")
            body_out.append("")
            continue

        # Detect ordered list (1. item)
        if re.match(r"^\d+\.\s+", line):
            body_out.append("\\begin{enumerate}")
            while i < n and re.match(r"^\d+\.\s+", lines[i].strip()):
                item_txt = re.sub(r"^\d+\.\s+", "", lines[i].strip())
                body_out.append(f"  \\item {format_inline_markdown(item_txt)}")
                i += 1
            body_out.append("\\end{enumerate}")
            body_out.append("")
            continue

        # Normal paragraph
        body_out.append(format_inline_markdown(line))
        body_out.append("")
        i += 1

    # Build bibliography block
    bib_lines = [
        "\\begin{thebibliography}{99}",
    ]
    for idx, r in enumerate(ref_items, 1):
        bib_lines.append(f"\\bibitem{{ref{idx}}} {r}")
        bib_lines.append("")
    bib_lines.append("\\end{thebibliography}")

    # Build full LaTeX document
    preamble = r"""\documentclass[journal,onecolumn]{IEEEtran}
\usepackage[utf8]{inputenc}
\usepackage[T1]{fontenc}
\usepackage{amsmath,amssymb,amsfonts}
\usepackage{graphicx}
\usepackage{booktabs}
\usepackage{array}
\PassOptionsToPackage{hyphens}{url}
\usepackage[hyphens]{url}
\usepackage[hidelinks,breaklinks=true]{hyperref}
\IfFileExists{xurl.sty}{\usepackage{xurl}}{}
\usepackage{microtype}

\emergencystretch=3em
\sloppy

\newsavebox{\tablebox}
\newcommand{\fittable}[1]{%
  \sbox{\tablebox}{#1}%
  \ifdim\wd\tablebox>\textwidth%
    \resizebox{\textwidth}{!}{\usebox{\tablebox}}%
  \else%
    \usebox{\tablebox}%
  \fi%
}

\begin{document}
% Force URL break points AFTER \begin{document} so IEEEtran's \AtBeginDocument{\urlstyle{same}} does not reset \UrlBreaks
\Urlmuskip=0mu plus 2mu\relax
\expandafter\def\expandafter\UrlBreaks\expandafter{\UrlBreaks\do\-\do\_\do\/\do\.\do\a\do\b\do\c\do\d\do\e\do\f\do\g\do\h\do\i\do\j\do\k\do\l\do\m\do\n\do\o\do\p\do\q\do\r\do\s\do\t\do\u\do\v\do\w\do\x\do\y\do\z}
"""
    title_block = f"""\\title{{{title}}}

\\author{{Moh.~Radithyo~Surya~Martuah%
\\thanks{{{author_footnote}}}}}

\\maketitle
"""

    if is_english:
        abs_block = f"""\\begin{{abstract}}
{abstract_en}
\\end{{abstract}}

\\begin{{IEEEkeywords}}
{keywords_en}
\\end{{IEEEkeywords}}
"""
    else:
        abs_block = f"""\\begin{{abstract}}
{abstrak_id}

\\vspace{{6pt}}
\\noindent\\textbf{{Kata Kunci:}} {keywords_id}

\\vspace{{6pt}}
\\noindent\\textit{{\\textbf{{Abstract---}}{abstract_en}}}

\\vspace{{4pt}}
\\noindent\\textit{{\\textbf{{Keywords:}} {keywords_en}}}
\\end{{abstract}}
"""

    full_tex = (
        preamble
        + title_block
        + abs_block
        + "\n"
        + "\n".join(body_out)
        + "\n"
        + "\n".join(bib_lines)
        + "\n\\end{document}\n"
    )
    return full_tex


def generate_bibtex_file(bib_path: Path):
    """Generate a standard BibTeX file for all 29 references."""
    bib_content = r"""@inproceedings{bakhtin2025network,
  author    = {Bakhtin, Alexander and Esposito, Matteo and Lenarduzzi, Valentina and Taibi, Davide},
  title     = {Network Centrality as a New Perspective on Microservice Architecture},
  booktitle = {Proceedings of the 2025 IEEE 22nd International Conference on Software Architecture (ICSA)},
  pages     = {72--83},
  year      = {2025},
  doi       = {10.1109/ICSA65012.2025.00017}
}

@misc{bremlerbarr2024performance,
  author       = {Bremler-Barr, Anat and Lavi, Ofek and Naor, Yotam and Rampal, Sanjeev and Tavori, Jonathan},
  title        = {Performance Comparison of Service Mesh Frameworks: The {mTLS} Test Case},
  howpublished = {arXiv:2411.02267},
  year         = {2024},
  doi          = {10.48550/arXiv.2411.02267}
}

@techreport{chandramouli2022devsecops,
  author      = {Chandramouli, Ramaswamy},
  title       = {Implementation of {DevSecOps} for a Microservices-Based Application with Service Mesh},
  institution = {National Institute of Standards and Technology},
  number      = {NIST SP 800-204C},
  year        = {2022},
  doi         = {10.6028/NIST.SP.800-204C}
}

@techreport{chandramouli2024dataprotection,
  author      = {Chandramouli, Ramaswamy and Hales, William},
  title       = {A Data Protection Approach for Cloud-Native Applications},
  institution = {National Institute of Standards and Technology},
  number      = {NIST IR 8505},
  year        = {2024},
  doi         = {10.6028/NIST.IR.8505}
}

@techreport{chandramouli2023zta,
  author      = {Chandramouli, Ramaswamy and Butcher, Zack},
  title       = {A Zero Trust Architecture Model for Access Control in Cloud-Native Applications in Multi-Location Environments},
  institution = {National Institute of Standards and Technology},
  number      = {NIST SP 800-207A},
  year        = {2023},
  doi         = {10.6028/NIST.SP.800-207A}
}

@article{cliff1993dominance,
  author  = {Cliff, Norman},
  title   = {Dominance Statistics: Ordinal Analyses to Answer Ordinal Questions},
  journal = {Psychological Bulletin},
  volume  = {114},
  number  = {3},
  pages   = {494--509},
  year    = {1993},
  doi     = {10.1037/0033-2909.114.3.494}
}

@article{dakic2024azure,
  author  = {Daki{\'c}, Vedran and Mori{\'c}, Zlatan and Kapulica, Ante and Regvart, Damir},
  title   = {Analysis of {Azure} Zero Trust Architecture Implementation for Mid-Size Organizations},
  journal = {Journal of Cybersecurity and Privacy},
  volume  = {5},
  number  = {1},
  pages   = {2},
  year    = {2024},
  doi     = {10.3390/jcp5010002}
}

@article{dhanapala2024performance,
  author  = {Dhanapala, Ishan and Bharti, Sourabh and McGibney, Alan and Rea, Susan},
  title   = {Toward a Performance-Based Trustworthy Edge-Cloud Continuum},
  journal = {IEEE Access},
  volume  = {12},
  pages   = {99201--99212},
  year    = {2024},
  doi     = {10.1109/ACCESS.2024.3429197}
}

@inproceedings{du2025generating,
  author    = {Du, Fan and Shi, J. and Chen, Q. and Li, L. and Guo, Minyi},
  title     = {Generating Microservice Graphs with Production Characteristics for Efficient Resource Scaling},
  booktitle = {Proceedings of the 2025 International Conference on Supercomputing (ICS '25)},
  year      = {2025},
  doi       = {10.1145/3721145.3725761}
}

@book{efron1993bootstrap,
  author    = {Efron, Bradley and Tibshirani, Robert J.},
  title     = {An Introduction to the Bootstrap},
  publisher = {Chapman \& Hall/CRC},
  year      = {1993}
}

@article{farhadighalati2025access,
  author  = {Farhadighalati, N. and Estrada-Jimenez, L. A. and Nikghadam-Hojjati, S. and Barata, J.},
  title   = {A Systematic Review of Access Control Models: Background, Existing Research, and Challenges},
  journal = {IEEE Access},
  volume  = {13},
  pages   = {17777--17806},
  year    = {2025},
  doi     = {10.1109/ACCESS.2025.3533145}
}

@article{freeman1977centrality,
  author  = {Freeman, Linton C.},
  title   = {A Set of Measures of Centrality Based on Betweenness},
  journal = {Sociometry},
  volume  = {40},
  number  = {1},
  pages   = {35--41},
  year    = {1977},
  doi     = {10.2307/3033543}
}

@article{gambo2025zta,
  author  = {Gambo, M. L. and Almulhem, A.},
  title   = {Zero Trust Architecture: A Systematic Literature Review},
  journal = {Journal of Network and Systems Management},
  volume  = {34},
  number  = {1},
  pages   = {25},
  year    = {2025},
  doi     = {10.1007/s10922-025-09998-x}
}

@inproceedings{geisberger2008betweenness,
  author    = {Geisberger, Robert and Sanders, Peter and Schultes, Dominik},
  title     = {Better Approximation of Betweenness Centrality},
  booktitle = {Proceedings of the 10th Workshop on Algorithm Engineering and Experiments (ALENEX)},
  pages     = {90--100},
  year      = {2008},
  doi       = {10.1137/1.9781611972887.9}
}

@article{haindl2024slr,
  author  = {Haindl, Philipp and Kochberger, Patrick and Sveggen, M.},
  title   = {A Systematic Literature Review of Inter-Service Security Threats and Mitigation Strategies in Microservice Architectures},
  journal = {IEEE Access},
  volume  = {12},
  pages   = {90252--90286},
  year    = {2024},
  doi     = {10.1109/ACCESS.2024.3406500}
}

@article{kruskal1952ranks,
  author  = {Kruskal, William H. and Wallis, W. Allen},
  title   = {Use of Ranks in One-Criterion Variance Analysis},
  journal = {Journal of the American Statistical Association},
  volume  = {47},
  number  = {260},
  pages   = {583--621},
  year    = {1952},
  doi     = {10.1080/01621459.1952.10483441}
}

@article{kumar2025microservice,
  author  = {Kumar, V.},
  title   = {Microservice-Driven Performance Optimization in Large-Scale Transaction Processing Systems},
  journal = {International Journal of Computational and Experimental Science and Engineering},
  volume  = {11},
  number  = {4},
  year    = {2025},
  doi     = {10.22399/ijcesen.4461}
}

@article{lakens2017equivalence,
  author  = {Lakens, Dani{\"e}l},
  title   = {Equivalence Tests: A Practical Primer for $t$ Tests, Correlations, and Meta-Analyses},
  journal = {Social Psychological and Personality Science},
  volume  = {8},
  number  = {4},
  pages   = {355--362},
  year    = {2017},
  doi     = {10.1177/1948550617697177}
}

@article{ma2025lightweight,
  author  = {Ma, Z. and Wei, H. and Jiang, J. and Wang, B. and Wang, H. and Di, Z.},
  title   = {A Lightweight Zero-Trust Authentication Architecture for {IoT} via Unified Enhanced {FAST-SM9} and Dynamic Re-Authentication},
  journal = {PLOS ONE},
  volume  = {20},
  number  = {10},
  pages   = {e0332943},
  year    = {2025},
  doi     = {10.1371/journal.pone.0332943}
}

@article{mann1947test,
  author  = {Mann, Henry B. and Whitney, Donald R.},
  title   = {On a Test of Whether One of Two Random Variables is Stochastically Larger than the Other},
  journal = {The Annals of Mathematical Statistics},
  volume  = {18},
  number  = {1},
  pages   = {50--60},
  year    = {1947},
  doi     = {10.1214/aoms/1177730491}
}

@inproceedings{romano2006appropriate,
  author    = {Romano, Jeanine and Kromrey, Jeffrey D. and Coraggio, Jesse and Skowronek, Jeff},
  title     = {Appropriate Statistics for Ordinal Level Data: Should We Really Be Using $t$-Test and {Cohen's} $d$ for Evaluating Group Differences on the {NSSE} and Similar Surveys?},
  booktitle = {Annual Meeting of the Florida Association of Institutional Research},
  pages     = {1--33},
  year      = {2006}
}

@techreport{rose2020zta,
  author      = {Rose, Scott and Borchert, Oliver and Mitchell, Stu and Connelly, Sean},
  title       = {Zero Trust Architecture},
  institution = {National Institute of Standards and Technology},
  number      = {NIST SP 800-207},
  year        = {2020},
  doi         = {10.6028/NIST.SP.800-207}
}

@inproceedings{samonte2024ehr,
  author    = {Samonte, Mary Jane C. and Aparize, J. E. R. and Geronimo, J. M. and Ori{\~n}o, C. C.},
  title     = {Implementing Zero Trust Security in Microservice Architecture of Electronic Health Record},
  booktitle = {2024 4th International Conference on Computer Systems (ICCS)},
  pages     = {98--105},
  year      = {2024},
  doi       = {10.1109/ICCS62594.2024.10795827}
}

@article{sasada2024webbiometrics,
  author  = {Sasada, Taisho and Taenaka, Yuzo and Kadobayashi, Youki and Fall, Doudou},
  title   = {Web-Biometrics for User Authenticity Verification in Zero Trust Access Control},
  journal = {IEEE Access},
  volume  = {12},
  pages   = {129611--129622},
  year    = {2024},
  doi     = {10.1109/ACCESS.2024.3413696}
}

@article{schuirmann1987tost,
  author  = {Schuirmann, Donald J.},
  title   = {A Comparison of the Two One-Sided Tests Procedure and the Power Approach for Assessing the Equivalence of Average Bioavailability},
  journal = {Journal of Pharmacokinetics and Biopharmaceutics},
  volume  = {15},
  number  = {6},
  pages   = {657--680},
  year    = {1987},
  doi     = {10.1007/BF01068419}
}

@article{shapiro1965normality,
  author  = {Shapiro, Samuel Sanford and Wilk, Martin B.},
  title   = {An Analysis of Variance Test for Normality (Complete Samples)},
  journal = {Biometrika},
  volume  = {52},
  number  = {3/4},
  pages   = {591--611},
  year    = {1965},
  doi     = {10.2307/2333709}
}

@article{verma2025zta,
  author  = {Verma, S.},
  title   = {Zero Trust Architecture in Cloud-Native Environments: Implementation Strategies \& Best Practices},
  journal = {International Journal of Computer Trends and Technology},
  volume  = {73},
  number  = {4},
  pages   = {114--122},
  year    = {2025},
  doi     = {10.14445/22312803/IJCTT-V73I4P114}
}

@book{walpole2020probability,
  author    = {Walpole, Ronald E. and Myers, Raymond H. and Myers, Sharon L. and Ye, Keying},
  title     = {Probability and Statistics for Engineers and Scientists},
  edition   = {9th},
  publisher = {Pearson},
  year      = {2020}
}

@article{zhang2025wasm,
  author  = {Zhang, Y. and Liu, M. and Wang, H. and Ma, Y. and Huang, G. and Liu, X.},
  title   = {Research on {WebAssembly} Runtimes: A Survey},
  journal = {ACM Transactions on Software Engineering and Methodology},
  volume  = {34},
  number  = {8},
  pages   = {Article 239},
  year    = {2025},
  doi     = {10.1145/3714465}
}
"""
    bib_path.write_text(bib_content, encoding="utf-8")


def main():
    id_md = BASE_DIR / "draft" / "naskah-artikel.md"
    en_md = BASE_DIR / "draft" / "en" / "naskah-artikel-en.md"

    id_tex = BASE_DIR / "draft" / "naskah-artikel.tex"
    en_tex = BASE_DIR / "draft" / "en" / "naskah-artikel-en.tex"
    bib_id = BASE_DIR / "draft" / "references.bib"
    bib_en = BASE_DIR / "draft" / "en" / "references.bib"

    tex_id_content = convert_markdown_to_latex(id_md, is_english=False)
    id_tex.write_text(tex_id_content, encoding="utf-8")
    print(f"[OK] Generated Indonesian LaTeX manuscript: {id_tex}")

    tex_en_content = convert_markdown_to_latex(en_md, is_english=True)
    en_tex.write_text(tex_en_content, encoding="utf-8")
    print(f"[OK] Generated English LaTeX manuscript   : {en_tex}")

    generate_bibtex_file(bib_id)
    generate_bibtex_file(bib_en)
    print(f"[OK] Generated BibTeX database            : {bib_id} & {bib_en}")


if __name__ == "__main__":
    main()
