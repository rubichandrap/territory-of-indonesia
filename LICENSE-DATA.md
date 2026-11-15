# Data license

The dataset produced by this repository (everything under `dist/`, and the
release assets published from it) is derived from OpenStreetMap data and is
made available under the
[Open Database License (ODbL) 1.0](https://opendatacommons.org/licenses/odbl/1-0/).

> © OpenStreetMap contributors.
> Data from [OpenStreetMap](https://www.openstreetmap.org/copyright), available
> under the Open Database License.

## What that means for you

- You may use, share and adapt the data, including commercially.
- You must credit OpenStreetMap. A visible "© OpenStreetMap contributors" on a
  map that uses the data is the usual way; see the
  [attribution guidelines](https://osmfoundation.org/wiki/Licence/Attribution_Guidelines).
- If you publish a database derived from this data, you must publish it under
  the ODbL as well (share-alike). A map image or an app that merely uses the
  data is a "produced work" and is not affected by share-alike.

The ODbL text above is the license; this page is a summary, not legal advice.

## Where the data comes from

The build reads a dated extract from [Geofabrik](https://download.geofabrik.de/).
`manifest.json` in every release records the file name, its SHA-256 and the
OpenStreetMap timestamp of the data, so any release can be traced back to the
exact input.

Kemendagri region codes, where present, are copied from OpenStreetMap tags
(`ref:ID:kemendagri`, `ref:kemendagri`, `ref`). This project does not add data
from other sources.
