# Territory of Indonesia
This is a dataset project that currently covering all territory of Indonesia from level 4 to level 6 of administration level. This dataset based on OSM Database that released in August 8th, 2022. This project generated using [territory-of-indonesia-toolkit](https://github.com/rubichandrap/territory-of-indonesia-toolkit).

## How to use
You probably only need all files in `./dist` directory to be using in your project. This dataset provides such important data, but there're two most important, which are `OSM ID` and `Coordinates`.

- OSM ID used for getting higher administration level file. For example, After you get the list of provinces, you can get `OSM ID` from one of the list item. Then use it to get the list of districts for that respective province, and so on.
- Coordinates used for making a polygon of boundaries in 3rd party application such Google Maps.

## How Coordinates can be so important
As I mentioned above, it used for making polygon of boundaries. If you are using Google Maps Service, it allows you to draw a polygon that represents an area in the cartesian plane. For example:

![polygon boundary](./assets/boundary-example.png)

## Why there are interface files
I hope, these interfaces can help when you have to Marshaling these jsons, into a struct or another data typed collections.