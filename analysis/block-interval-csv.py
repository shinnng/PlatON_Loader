#!/usr/bin/env python

import json
import sys
import urllib.request as request
import plotly.graph_objects as go
import pandas as pd


def max_block_number(url):
    body = {"aggs": {"max": {"max": {"field": "number"}}}}

    req = request.Request(
        url=url,
        method="POST",
        data=json.dumps(body).encode("utf8"),
        headers={"content-type": "application/json"},
    )
    with request.urlopen(req) as f:
        data = f.read()
        objs = json.loads(data)
        return objs["aggregations"]["max"]["value"]
    return 0


def block_interval(url, tfrom):
    if tfrom == 0:
        tfrom = 1
    body = {
        "size": 1000,
        "from": 0,
        "_source": "interval",
        "query": {"bool": {"must": [{"range": {"number": {"gt": tfrom}}}]}},
    }

    req = request.Request(
        url=url,
        method="POST",
        data=json.dumps(body).encode("utf8"),
        headers={"content-type": "application/json"},
    )

    l = []
    #print(json.dumps(body).encode("utf8"))
    with request.urlopen(req) as f:
        data = f.read()
        objs = json.loads(data)

        hits = objs["hits"]["hits"]

        for doc in hits:
            l.append(doc["_source"]["interval"])

    return l


def run(host, indexname, output):
    url = "{}/{}/_search".format(host, indexname)

    max_number = max_block_number(url)
    if max_number == 0:
        print("max block number is 0")
        return

    tfrom = 0
    l = []
    while tfrom < max_number:
        ll = block_interval(url, tfrom)
        tfrom = tfrom+1000

        for val in ll:
            l.append(val)

    # draw(x, y, "./test.png")

    fp = open(output, "wb")
    fp.write("sequence,interval\n".encode("utf8"))
    i = 0
    for interval in l:
        s = "{},{}\n".format(i, interval)
        fp.write(s.encode("utf8"))
        i = i + 1
    fp.close()

    #draw(img_output, output)


def draw(img_file, csv_file):
    data = pd.read_csv(csv_file)
    fig = go.Figure(
        data=go.Scatter(x=data["sequence"], y=data["interval"], mode="markers")
    )
    fig.update_layout()
    fig.write_image(img_file, width=1600, height=700, scale=1.0)


if __name__ == "__main__":
    if len(sys.argv) < 4:
        print(
            "{} <url> <indexname> <output csv>".format(sys.argv[0])
        )
        sys.exit(0)
    run(sys.argv[1], sys.argv[2], sys.argv[3])
