# Territory of Indonesia

Administrative boundaries of Indonesia as GeoJSON, from provinces down to
villages, built from [OpenStreetMap](https://www.openstreetmap.org/) with a
reproducible pipeline that lives in this repository.

This is **v2**. It replaces the 2022 dataset and its separate generator
(`territory-of-indonesia-toolkit`), which are now one project. v1 is kept at
the `v1` tag; see [Migrating from v1](#migrating-from-v1).

**[Explore it on a map](https://rubichandrap.github.io/territory-of-indonesia/)**: click a
province, then a regency, a district and a village. It runs entirely in your browser
from the hosted files, with no server and no map library, and each click downloads
only the next small file.

## What is in it

Data as of **2026-11-14** (OpenStreetMap):

| Level | OSM `admin_level` | Regions | Coverage |
|---|:-:|--:|---|
| `province` (provinsi) | 4 | 38 | all of them |
| `regency` (kabupaten / kota) | 5 | 514 | all of them (415 kabupaten, 97 kota, 2 unclassified) |
| `district` (kecamatan / distrik) | 6 | 2,177 | partial, about a third of Indonesia |
| `village` (desa / kelurahan) | 7 | 6,534 | partial, under a tenth of Indonesia |

OpenStreetMap is complete down to the kabupaten/kota level. Below that it is
being mapped region by region, so the district and village files cover only
part of the country. The official totals are roughly 7,000 kecamatan and
83,000 desa/kelurahan (approximate figures). Each release's `report.md` states
exactly what it contains and what looked wrong.

## Getting the data

Each release is published on the
[Releases page](https://github.com/rubichandrap/territory-of-indonesia/releases)
as one zip per tier, plus `manifest.json` and `report.json`:

| Asset | Zip | Unzipped | Use it for |
|---|--:|--:|---|
| `territory-of-indonesia-index.zip` | 0.7 MB | 2.4 MB | dropdowns, search, map markers: properties, bounding box and a label point for every region, **no geometry** |
| `territory-of-indonesia-simplified.zip` | 4 MB | 15 MB | drawing maps: GeoJSON with simplified geometry |
| `territory-of-indonesia-full.zip` | 57 MB | 186 MB | analysis: GeoJSON with the geometry exactly as OpenStreetMap has it |

GitHub Releases cannot serve single files, so for a copy you can fetch file by
file, use the hosted one below, or unzip a tier and host it yourself.

### Hosted copy

The newest release's `index` and `simplified` tiers are also served from
GitHub Pages, with CORS enabled, so a web page can fetch them directly:

```
https://rubichandrap.github.io/territory-of-indonesia/v2/index/provinces.json
https://rubichandrap.github.io/territory-of-indonesia/v2/simplified/provinces.geojson
https://rubichandrap.github.io/territory-of-indonesia/v2/manifest.json
```

The path after the site root is the layout below, behind a `v2/` that is the
format version, not a date. The site always holds the **newest** data, so a
file can change under the same URL. To pin a version, download that release's
zip. The `full` tier is only in the releases.

### Layout

```
manifest.json
index/provinces.json                  geometry-free entries
index/regencies/<province id>.json    the regencies of one province
index/districts/<regency id>.json
index/villages/<district id>.json
simplified/provinces.geojson          FeatureCollections, same file names
simplified/regencies/<province id>.geojson
full/...
```

Walk down the tree with the `osm_id` of what the user picked. Each index entry
has a `children` object (`{"districts": 12}`) that says which files exist for
it:

```js
const base = "https://rubichandrap.github.io/territory-of-indonesia/v2";
const get = async (path) => (await fetch(`${base}/${path}`)).json();

const provinces = await get("index/provinces.json");
const jabar = provinces.find((p) => p.name === "Jawa Barat");

const regencies = await get(`index/regencies/${jabar.osm_id}.json`);
const bandung = regencies.find((r) => r.official_name === "Kota Bandung");

// Only some regions have districts mapped, so check `children` first.
if (bandung.children?.districts) {
  const districts = await get(`index/districts/${bandung.osm_id}.json`);
}
```

To draw a region, take the same path in `simplified/` (with `.geojson` instead
of `.json`) and give the feature to Leaflet, MapLibre, Google Maps
(`Data.addGeoJson`) or any GeoJSON consumer.

### Properties

Every feature's `properties`, and every index entry, has:

| Property | Always | Meaning |
|---|:-:|---|
| `osm_id` | yes | OpenStreetMap relation id, unique across the dataset |
| `level` | yes | `province`, `regency`, `district` or `village` |
| `admin_level` | yes | 4, 5, 6 or 7 |
| `name` | yes | OSM `name`: the local name, usually without the "Kabupaten"/"Kecamatan" prefix |
| `official_name` | | name with the prefix, such as `Kota Bandung` |
| `name_en` | | English name |
| `kind` | | `kabupaten`, `kota`, `kecamatan`, `distrik`, `desa`, `kelurahan`, ... See below |
| `code` | | Kemendagri region code (`32`, `32.73`, `32.73.01`, `32.73.01.1001`) |
| `wikidata` | | Wikidata id |
| `parent_id` | | the nearest ancestor in the dataset; absent for provinces |
| `ancestors` | | every ancestor, province first |

Index entries add `bbox` (`[west, south, east, north]`), `center` (a point
inside the region, `[lon, lat]`) and `children`. Features carry `id`
(same as `osm_id`) and `bbox`. The contract is
[`schema/territory.schema.json`](schema/territory.schema.json); TypeScript
types are in [`types/territory.d.ts`](types/territory.d.ts).

`code` is present only where OpenStreetMap has it: all provinces, and about
41% of regencies, 38% of districts and 26% of villages. `kind` is read from
`official_name`/`name` and falls back to the Kemendagri numbering rules
(kota codes are `NN.71` and up; a village code starting `1` is a kelurahan, `2`
a desa), so it is empty where neither says anything.

### Things to know

- **A level can be missing.** `parent_id` is the nearest ancestor that exists in
  the dataset, and at most one level is skipped. 768 villages sit directly
  under a regency because their kecamatan is not mapped; their file is
  `index/villages/<regency id>.json`. Compare `admin_level` with the parent's
  to tell.
- **`simplified` is for drawing.** It keeps 8% of the points; the median area
  error against `full` is 0.4%, and a village a hundred metres across can be
  off by more. Simplification is per ring, so a border shared by two regions can
  show a hairline gap. Tiny islands are kept.
- **Names come from OpenStreetMap** and follow its conventions, which vary by
  region. Use `official_name` or `code` where you need a stable label.

## Migrating from v1

v2 is a clean break. Nothing from v1 is served any more; v1 stays available at
the [`v1` tag](https://github.com/rubichandrap/territory-of-indonesia/tree/v1).

| | v1 | v2 |
|---|---|---|
| Distribution | files committed under `dist/` | release assets, built by CI |
| Levels | provinces, "districts" (= regencies), "sub_districts" (= kecamatan) | provinces, regencies, districts, **villages** |
| `osm_id` | negative (`-304751`) | the real, positive relation id |
| Names | `name` was English-first, plus `local_name` | `name` is the OSM name; `name_en`, `official_name` added; `local_name` removed |
| Parents | `parents`: a comma-separated string | `parent_id` and `ancestors` |
| Removed | `boundary` (always `administrative`) | |
| Added | | `level`, `kind`, `code`, `wikidata`, `bbox`, `center`, `children` |
| Types | hand-written Go, Dart, TS (out of step with the data) | one JSON Schema, checked against real output by a test |

v2 also fixes data bugs in v1. About 55 kecamatan whose kabupaten was missing
from the source were filed under their province id, so a lookup by kabupaten
never found them (Kota Bandung's kecamatan, for one); 46 of the ones that
exist in both versions now have their real parent. And 97 of v1's 454
"regencies" were islands and national parks that OpenStreetMap had tagged
`admin_level=5` on closed ways. v2 keeps only boundary relations. (One more,
Demak, is a real kabupaten whose relation was re-created under a new id.)

## Building it yourself

You need Go 1.27 or newer and either
[osmium-tool](https://osmcode.org/osmium-tool/) (`apt install osmium-tool`) or
Docker. With Docker, the image is built on first use from
[`internal/osmium/Dockerfile`](internal/osmium/Dockerfile) and has osmium 1.18.
Ubuntu 24.04's package (1.16), which CI installs, was tested against the same
snapshot and produces a byte-identical dataset.

```sh
go run ./cmd/toi all
```

That downloads the newest Geofabrik extracts (1.7 GB for Indonesia, 250 MB for
the neighbour) into `.cache/`, assembles the boundaries, and writes the dataset
to `dist/`. It takes about a minute plus the download. The steps also run
separately:

```sh
toi fetch    -snapshot 261114            # a dated snapshot instead of "latest"
toi extract  -pbf a.osm.pbf -pbf b.osm.pbf -osmium docker
toi build    -out dist                   # also writes dist/report.md and report.json
toi validate -dist dist -baseline previous/manifest.json
```

`toi validate` exits 1 on errors and is what CI uses as a gate. Given the
`manifest.json` of the previous release as `-baseline`, it also fails if any
level loses more than 2% of its regions.

### How it works

1. **fetch** downloads dated extracts from Geofabrik and checks the md5 they
   publish.
2. **extract** keeps `boundary=administrative` relations with `osmium
   tags-filter`, merges the extracts, keeps `admin_level` 4 to 7, and lets
   `osmium export` assemble the member ways into polygons. Assembling rings
   from hundreds of ways is the hard part, and osmium's assembler is the
   reference implementation.
3. **build** works out the hierarchy. OpenStreetMap does not store it usably, so
   it is found geometrically: each region's interior point is tested against
   the polygons one level up, then two levels up, and so on. If several
   parents contain the point (an enclave inside a surrounding polygon), the
   parent whose Kemendagri code prefixes the region's own code wins, else the
   smallest. At most one level may be skipped. Regions that no Indonesian
   province contains are dropped (the extracts include parts of Malaysia,
   Thailand and the Philippines), and so are orphans: regions inside a province
   that nothing within a skipped level contains.
4. **validate** re-reads the files and checks them independently of the build:
   file layout against the index, referential integrity, children counts,
   ancestor chains, geometry validity and bounds, manifest counts.

**Why two extracts?** Geofabrik cuts the Indonesia extract at the border, so a
boundary that shares ways with Malaysia (Kalimantan Utara, Kepulauan Riau,
Nunukan) cannot be assembled from it alone: with the Indonesia extract by
itself the build had 36 of 38 provinces, 502 of 514 regencies, 2,157 districts
instead of 2,177 and 6,483 villages instead of 6,548. Use
`-neighbor none` to skip it, or `-neighbor` to name others.

**Every build records its inputs.** `manifest.json` lists each extract with its
URL, SHA-256 and OSM timestamp, so a release can be traced to the exact input.
The same inputs give byte-identical output.

### Known limitations

- 16 villages (0.2%) have a Kemendagri code that does not fit the parent their
  polygon falls in. That is an inconsistency inside OpenStreetMap, not
  something the build can settle. They are listed in `report.json`.
- 14 "villages" are about a hundred metres across and lie in the sea off the
  Gunungkidul coast (Drini, Timang, Watupanjang and other beaches), outside
  every regency polygon. They are mis-tagged in OpenStreetMap and are left out;
  `report.json` lists them as orphans.
- A relation OpenStreetMap has but osmium cannot turn into a polygon (a ring
  that does not close) is missing from the dataset. One Indonesian village,
  Kedungsari, is in this state today. `report.json` lists them.

## Development

```sh
go vet ./... && go test -race ./...
```

The tests include a contract test that validates real emitted files against
`schema/territory.schema.json`, and a guard that fails if
`types/territory.d.ts` is missing a property the schema declares. CI
([`ci.yml`](.github/workflows/ci.yml)) runs them on every push.
[`release-data.yml`](.github/workflows/release-data.yml) builds and publishes
the dataset monthly.

Layout: `cmd/toi` is the command line; `internal/` has one package per step
(`fetch`, `osmium`, `source`, `hierarchy`, `emit`, `validate`, `report`) plus
`geom`, `geojson` and `model`. `site/index.html` is the landing page and map
explorer of the hosted copy: one self-contained file, with no dependencies. A
Go test checks that every dataset link in it exists. `site/e2e/run.sh` drives the
page in a headless Chrome against a real `dist/` (hover, clicking down to a
village, Back, search, shared links, dark mode, a phone); run it after changing
the page.

### Publishing

`release-data.yml` does both jobs: it publishes the release and deploys the
Pages site. Enable Pages once under Settings > Pages > Build and deployment >
Source: **GitHub Actions**. Run the workflow by hand to build a dated snapshot;
untick *Publish a GitHub release* to update only the Pages site. The site is
only updated for the `latest` snapshot, so rebuilding an old one cannot
overwrite newer data.

## License

The code is [MIT](LICENSE). The data is derived from OpenStreetMap and is
licensed under the [ODbL 1.0](https://opendatacommons.org/licenses/odbl/1-0/):
**© OpenStreetMap contributors.** See [LICENSE-DATA.md](LICENSE-DATA.md) for
what that asks of you.
