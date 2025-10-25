# 🐐 Dominions 6 API by Monkeydew

A fast in-memory **SQLite-powered API** and **web scraper** for _Dominions 6 Inspector_ data.  
Built with Go + Playwright.  
Scrapes all game data (units, items, spells, sites, mercs, events), captures their icons, and exposes a queryable JSON REST API.

---

## Launch Modes

### Build Mode (with scraping)

Fetches the **latest data** from the [dom6inspector GitHub](https://github.com/larzm42/dom6inspector) repo, runs a local web server, scrapes everything, stores it in SQLite, and then launches the API.

dom6api.exe build
Clones the inspector repo

Starts a local Python webserver (localhost:8001)

Waits until it’s live

Uses Playwright to render and extract all entities

Saves PNGs and data into Data/

Populates the database

Then starts the API at:
http://localhost:8002

No-Build Mode (API only)
Skips scraping and launches only the API using existing data.

dom6api.exe

How Scraping Works
Launches a headless Chromium instance via Playwright.

Loads the inspector web app (http://localhost:8001).

Iterates over each data category:

events, items, mercs, sites, spells, units

Sorts entities by ID and renders overlays via the in-page DMI.modctx.

Extracts key fields per category (ID, name, etc.).

Captures PNG screenshots of rendered overlays.

Inserts structured data into Data/dom6api.db.

Outputs logs like:

2025/10/25 12:36:04 /items/1/screenshot

Each table mirrors the inspector data:

Table Columns
items id, name, type, constlevel, mainlevel, mpath, gemcost
spells id, name, gemcost, mpath, type, school, researchlevel
units id, name, hp, size, mountmnr, coridermnr
mercs id, name, bossname, com, unit, nrunits
sites id, name, path, level, rarity
events id, name

All tables also include a generated image path, e.g.
Data/spells/123.png

## API Usage

Query by ID
/spells/1

/spells/?id=123
Query by Column

/units/?name=archer
Fuzzy Search
Enable fuzzy and partial matching:

/spells/?name=bless&match=fuzzy

Matches even if names differ slightly

images:  
table/id/screenshot  
returns image

## Example Response

{
"spells": [
{
"id": 42,
"name": "Bless",
"school": "Enchantment",
"gemcost": 1,
"image": "Data/spells/42.png"
}
]
}

## Compile

Standard:  
go build -o dom6api.exe .

Optimized (for the cool kids):  
build -trimpath -ldflags="-s -w" -o dom6api.exe .;upx --best --lzma dom6api.exe

## Warning ⚠️

This software was built and tested by an idiot.

## 🐐 Credits 🐐

Big thanks to Tim Nordenfur for [dom5api](https://github.com/gtim/dom5api) — a huge source of inspiration and the defacto predicessor

Created by Monkeydew.
Use, modify, and distribute freely.
MIT Licensed.
