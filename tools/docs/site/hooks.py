"""MkDocs hook for the Kiestoon manual.

Screenshot shorthand: ![Caption](shot:name) (optionally followed by { width=320 }) becomes the light and the dark screenshot of that page in the
language of the page. The site shows the one that matches the reader's light/dark choice (Material's
#only-light / #only-dark), and a click opens it full size.
"""
import os
import re

SHOT = re.compile(r"!\[([^\]]*)\]\(shot:([a-z0-9-]+)\)(\{[^}]*\})?")


def on_page_markdown(markdown, page, config, files, **kwargs):
    src = page.file.src_uri
    lang = "nl" if page.file.abs_src_path and page.file.abs_src_path.endswith(".nl.md") else "en"
    up = "../" * src.count("/")

    def shot(m):
        caption, name = m.group(1), m.group(2)
        extra = (m.group(3) or "{}")[1:-1].strip()
        attrs = "{ loading=lazy " + (extra + " " if extra else "") + "}"
        base = f"{up}assets/screenshots/{lang}"
        return (
            f"![{caption}]({base}/light/{name}.png#only-light){attrs}\n"
            f"![{caption}]({base}/dark/{name}.png#only-dark){attrs}"
        )

    return SHOT.sub(shot, markdown)
