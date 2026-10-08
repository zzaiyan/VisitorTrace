#!/usr/bin/env python3
"""Generate the built-in place-name index from public geographic datasets."""

import argparse
import gzip
import json
import re
import unicodedata
from collections import Counter, defaultdict
from pathlib import Path


def city_key(value: str) -> str:
    output = []
    pending_space = False
    for char in value.strip():
        if "！" <= char <= "～":
            char = chr(ord(char) - ord("！") + ord("!"))
        if char in {"'", "’", "ʼ", "."}:
            continue
        if unicodedata.category(char).startswith(("L", "N")):
            if pending_space and output:
                output.append(" ")
            output.append(char.lower())
            pending_space = False
        else:
            pending_space = True
    return "".join(output)


def has_han(value: str) -> bool:
    return any("\u3400" <= char <= "\u4dbf" or "\u4e00" <= char <= "\u9fff" for char in value)


SUFFIXES = (
    " of the peoples republic of china", " special administrative region", " prefecture level city",
    " autonomous region", " municipality", " prefecture", " province", " region", " city", " sar", " china",
    "特别行政区", "特別行政區", "维吾尔自治区", "壮族自治区", "回族自治区", "自治区", "自治州", "地区", "省级", "省", "市", "盟",
)


def alias_variants(value: str) -> set[str]:
    result = {value}
    changed = True
    while changed:
        changed = False
        for item in list(result):
            for suffix in SUFFIXES:
                stripped = item[: -len(suffix)] if suffix and item.endswith(suffix) else ""
                if stripped and stripped not in result:
                    result.add(stripped)
                    changed = True
    return result


def place_rank(feature_code: str, population: str) -> int:
    weights = {
        "PPLC": 8,
        "PPLA": 7,
        "PPLA2": 6,
        "PPLA3": 5,
        "PPLA4": 4,
        "PPLX": 3,
        "PPL": 2,
    }
    try:
        people = min(int(population), 1_000_000_000)
    except ValueError:
        people = 0
    return weights.get(feature_code, 1) * 100_000 + people


def canonical_city_name(name: str, candidates: list[str], ascii_name: str) -> str:
    counts = Counter(city_key(value) for value in candidates if value)
    preferred = city_key("".join(
        char for char in unicodedata.normalize("NFD", name)
        if not unicodedata.combining(char)
    ))

    def score(value: str) -> tuple[int, int, int, int, int, int]:
        title_case = value == value.title()
        ascii_title = value.isascii() and title_case
        return (
            int(city_key(value) == preferred),
            int(ascii_title and counts[city_key(value)] > 1),
            int(ascii_title),
            counts[city_key(value)],
            int(value == ascii_name),
            -len(value),
        )

    return max((value for value in candidates if value), key=score)



def split_tab(line: str) -> list[str]:
    return line.rstrip("\n").split("\t")


def load_iso_subdivisions(path: Path) -> dict[str, dict[str, dict[str, str]]]:
    result: dict[str, dict[str, dict[str, str]]] = defaultdict(dict)
    with path.open(encoding="utf-8") as source:
        for item in json.load(source):
            country = item["country"]
            for name in {item.get("name", ""), item.get("name_en", "")}:
                key = city_key(name)
                if key:
                    result[country][key] = {
                        "code": item["code"].split("-", 1)[-1],
                        "name": item.get("name_en") or item.get("name") or name,
                    }
    return result


def load_admin1(path: Path, iso: dict[str, dict[str, dict[str, str]]]) -> dict[str, dict[str, str]]:
    result = {}
    with path.open(encoding="utf-8") as source:
        for line in source:
            fields = split_tab(line)
            if len(fields) < 4:
                continue
            code, name, _, geoname_id = fields[:4]
            country, admin_code = code.split(".", 1)
            subdivision = iso.get(country, {}).get(city_key(name))
            if not subdivision:
                continue
            result[geoname_id] = {
                "country": country,
                "geoname_code": code,
                "admin_code": admin_code,
                "name": name,
                "iso_code": subdivision["code"],
                "iso_name": subdivision["name"],
            }
            result[code] = result[geoname_id]
    return result


def admin_aliases(path: Path, admin1: dict[str, dict[str, str]]) -> dict[str, set[str]]:
    result: dict[str, set[str]] = defaultdict(set)
    with path.open(encoding="utf-8") as source:
        for line in source:
            fields = split_tab(line)
            if len(fields) < 4 or fields[1] not in admin1:
                continue
            value = fields[3]
            if has_han(value):
                result[fields[1]].add(value)
    return result


def compact_city_targets(targets: list[dict[str, str]]) -> list[dict[str, str]]:
    if not targets:
        return []
    by_country: dict[str, list[dict[str, str]]] = defaultdict(list)
    for target in targets:
        by_country[target["country"]].append(target)
    compact = []
    for country in sorted(by_country):
        unique = {}
        for target in by_country[country]:
            identity = (target["city"], target["region_code"], target["region_name"])
            if identity not in unique or int(target["rank"]) > int(unique[identity]["rank"]):
                unique[identity] = target
        best = max(unique.values(), key=lambda target: (int(target["rank"]), target["city"], target["region_code"]))
        compact.append({key: value for key, value in best.items() if key != "rank"})
    return compact


def generate(args: argparse.Namespace) -> None:
    iso = load_iso_subdivisions(args.subdivisions)
    admin1 = load_admin1(args.admin1, iso)
    aliases = admin_aliases(args.alternate_names, admin1)
    city_targets: dict[str, list[dict[str, str]]] = defaultdict(list)
    region_targets: dict[str, list[dict[str, str]]] = defaultdict(list)

    for geoname_id, values in aliases.items():
        admin = admin1[geoname_id]
        target = {
            "country": admin["country"],
            "region_code": admin["iso_code"],
            "region_name": admin["iso_name"],
        }
        for value in values | {admin["name"]}:
            region_targets[city_key(value)].append(target)

    with args.cities.open(encoding="utf-8") as source:
        for line in source:
            fields = split_tab(line)
            if len(fields) < 15:
                continue
            _, name, ascii_name, alternate_names, _, _, feature_class, feature_code, country, _, admin1_code, _, _, _, population, *_ = fields[:19]
            if feature_class != "P" or not re.fullmatch(r"PPL[A-Z0-9]*|PPLC|PPLX|PPLS", feature_code):
                continue
            admin = admin1.get(f"{country}.{admin1_code}", {})
            region_name = admin.get("iso_name", "")
            region_code = admin.get("iso_code", "")
            candidates = [ascii_name or name, name, *alternate_names.split(",")]
            han_aliases = {value for value in candidates if value and has_han(value)}
            if not han_aliases:
                continue
            canonical = canonical_city_name(name, candidates, ascii_name)
            target = {
                "country": country,
                "city": canonical,
                "region_code": region_code,
                "region_name": region_name,
                "rank": str(place_rank(feature_code, population)),
            }
            for value in han_aliases | {canonical, name, ascii_name}:
                if not value:
                    continue
                for alias in alias_variants(value):
                    city_targets[city_key(alias)].append(target)

    compact_cities = {key: compact_city_targets(targets) for key, targets in city_targets.items()}
    compact_regions: dict[str, list[dict[str, str]]] = {}
    for key, targets in region_targets.items():
        by_country = defaultdict(list)
        for target in targets:
            by_country[target["country"]].append(target)
        values = []
        for country in sorted(by_country):
            unique = {json.dumps(target, sort_keys=True) for target in by_country[country]}
            if len(unique) == 1:
                values.append(by_country[country][0])
        if values:
            compact_regions[key] = values

    data_lines = []
    for key in sorted(compact_cities):
        for target in compact_cities[key]:
            data_lines.append("\t".join(["city", key, *(target[field] for field in ("country", "city", "region_code", "region_name"))]))
    for key in sorted(compact_regions):
        for target in compact_regions[key]:
            data_lines.append("\t".join(["region", key, *(target[field] for field in ("country", "region_code", "region_name"))]))
    data = gzip.compress(("\n".join(data_lines) + "\n").encode("utf-8"), compresslevel=9, mtime=0)
    args.data_output.write_bytes(data)
    args.output.write_text(
        "package geoip\n\n"
        "// Code generated by tools/generate-place-names.py. Do not edit manually.\n\n"
        "import _ \"embed\"\n\n"
        "//go:embed place_names.tsv.gz\n"
        "var placeNamesData []byte\n",
        encoding="utf-8",
    )
    print(f"generated {len(compact_cities)} city aliases and {len(compact_regions)} region aliases")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("cities", type=Path)
    parser.add_argument("admin1", type=Path)
    parser.add_argument("alternate_names", type=Path)
    parser.add_argument("subdivisions", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("data_output", type=Path)
    generate(parser.parse_args())


if __name__ == "__main__":
    main()
