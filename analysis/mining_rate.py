#!/usr/bin/env python

import json
import sys
import urllib.request as request

default_epoch = 1
amount = 10


class MiningRate:
    def __init__(self, url, indexname, num_validators):
        self.url = url
        self.indexname = indexname
        self.num_validators = int(num_validators)

    def _last_epoch(self):
        body = {
            "query": {"bool": {"must": [{"term": {"type": "block"}}]}},
            "size": 1,
            "sort": [{"timestamp": {"order": "desc"}}],
        }
        url = "{}/{}/_search".format(self.url, self.indexname)

        res = self._post(url, body)
        if res:
            hits = res["hits"]["hits"]
            if len(hits) > 0:
                doc = res["hits"]["hits"][0]["_source"]
                return doc["epoch"]
        return 0

    def _agg_epoch(self, epoch):
        body = {
            "query": {
                "bool": {
                    "must": [{"term": {"epoch": epoch}}, {"term": {"type": "block"}}]
                }
            },
            "size": 0,
            "aggs": {
                "view": {
                    "terms": {"field": "view", "size": 1000, "order": {"_key": "asc"}}
                }
            },
        }
        url = "{}/{}/_search".format(self.url, self.indexname)

        res = self._post(url, body)
        if res:
            return res["aggregations"]["view"]["buckets"]
        return None

    def _get_validator(self, epoch, view):
        body = {
            "query": {
                "bool": {
                    "must": [
                        {"term": {"type": "validator"}},
                        {"term": {"epoch": epoch}},
                        {"term": {"index": view}},
                    ]
                }
            }
        }

        url = "{}/{}/_search".format(self.url, self.indexname)

        res = self._post(url, body)
        if res:
            hits = res["hits"]["hits"]
            if len(hits) > 0:
                return hits[0]["_source"]
        return None

    def _post(self, url, body):
        req = request.Request(
            url=url,
            method="POST",
            data=json.dumps(body).encode("utf8"),
            headers={"content-type": "application/json"},
        )

        with request.urlopen(req) as f:
            data = f.read()
            objs = json.loads(data)
            return objs
        return None

    def _uncompleted_view(self, epoch, views):
        view_mine = dict()
        for i in range(self.num_validators):
            view_mine[i] = {"view": i, "count": 0, "nodeID": "None", "host": "None"}

        for view in views:
            view_number = view["key"]
            index = view_number % self.num_validators
            v = view_mine[index]
            v["count"] = v["count"] + view["doc_count"]
            view_mine[index] = v

        uncompleted = []
        for _, view in view_mine.items():
            if view["count"] < amount:
                validator = self._get_validator(epoch, view["view"])
                if validator:
                    view["nodeID"] = validator["nodeID"]
                    view["host"] = validator["host"]
                uncompleted.append(view)
        return uncompleted

    def uncompleted_mining_rate(self):
        last_epoch = self._last_epoch()
        uncompleted = dict()
        for i in range(default_epoch + 1, last_epoch, 1):
            aggs = self._agg_epoch(i)

            views = self._uncompleted_view(i, aggs)
            if views:
                uncompleted[i] = views

        for epoch, views in uncompleted.items():
            uncompleted[epoch] = sorted(views, key=lambda k: k["count"], reverse=True)

        print(
            "|{:<10} | {:<10} | {:<10}| {:<20} | {:<10} |".format(
                "epoch", "view", "nodeID", "host", "count"
            )
        )

        for epoch, views in uncompleted.items():
            for view in views:
                print(
                    "|{:<10} | {:<10} | {:<10}| {:<20} | {:<10} |".format(
                        epoch,
                        view["view"],
                        view["nodeID"][:8],
                        view["host"],
                        view["count"],
                    )
                )

    def _validators(self, last_epoch):
        body = {
            "query": {
                "bool": {
                    "must": [
                        {"term": {"type": "validator"}},
                        {"range": {"epoch": {"gt": 1, "lt": last_epoch}}},
                    ]
                }
            },
            "size": 0,
            "aggs": {"vds": {"terms": {"field": "nodeID.keyword", "size": 1000}}},
        }
        url = "{}/{}/_search".format(self.url, self.indexname)

        res = self._post(url, body)

        if res:
            buckets = res["aggregations"]["vds"]["buckets"]
            vds = dict()
            for b in buckets:
                vds[b["key"]] = {
                    "nodeID": b["key"],
                    "rounds": b["doc_count"],
                    "count": 0,
                }
            return vds
        return None

    def _num_blocks(self, nodeid, last_epoch):
        body = {
            "query": {
                "bool": {
                    "must": [
                        {"term": {"type": "block"}},
                        {"term": {"node_id": nodeid}},
                        {"range": {"epoch": {"gt": 1, "lt": last_epoch}}},
                    ]
                }
            },
            "size": 0,
        }

        url = "{}/{}/_search".format(self.url, self.indexname)

        res = self._post(url, body)
        if res:
            return res["hits"]["total"]["value"]
        return 0

    def _validator(self, nodeid):
        body = {
            "query": {
                "bool": {
                    "must": [
                        {"term": {"type": "validator"}},
                        {"term": {"nodeID": nodeid}},
                    ]
                }
            },
            "size": 1,
        }
        url = "{}/{}/_search".format(self.url, self.indexname)
        res = self._post(url, body)
        if res:
            return res["hits"]["hits"][0]["_source"]["host"]
        return None

    def _fill_mining(self, vds, last_epoch):
        new_vds = dict()
        for k, vd in vds.items():
            c = self._num_blocks(k, last_epoch)
            h = self._validator(k)
            vd["count"] = c
            vd["host"] = h
            rate = vd["count"] / (vd["rounds"] * 10)
            vd["rate"] = rate * 100
            new_vds[k] = vd

        return new_vds

    def _rate(self):
        last_epoch = self._last_epoch()
        vds = self._validators(last_epoch)
        if vds is None:
            return

        vds = self._fill_mining(vds, last_epoch)

        sorted_vds = sorted(vds.items(), key=lambda k: k[1]["rate"], reverse=True)

        print(
            "|{:<11} | {:<20} | {:<10} | {:<10} | {:<10} |".format(
                "NodeID", "Host", "Rounds", "Count", "Rate(%)"
            )
        )
        for k, v in sorted_vds:
            print(
                "| {:<10} | {:<20} | {:<10} | {:<10} | {:<10.2f} |".format(
                    v["nodeID"][:8], v["host"], v["rounds"], v["count"], v["rate"]
                )
            )

    def rate(self):
        # 1. get the validator list
        # 2. get the number of blocks of a validator mining
        # 3. get the rounds
        #
        self._rate()


if __name__ == "__main__":
    if sys.argv[1] == "uncomplete":
        mr = MiningRate(*sys.argv[2:])
        mr.uncompleted_mining_rate()
    elif sys.argv[1] == "rate":
        mr = MiningRate(*sys.argv[2:])
        mr.rate()
    else:
        print("unexpected cmd %s" % (sys.argv[1]))
