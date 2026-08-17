# Storage
Each S3-Object is split into **chunks** which are then split into **shards**, the shards are then encoded with **reed-solomon**  
encoding and stored across multiple **backends**. The Objects are split in a way that guarantees a variable amount of backend failures  
before data is lost. 

## Chunks

## Shards

## Backends
Backend instances are declared in `backends.yaml` (path configurable via `SHARDS3_BACKENDS_CONFIG_PATH`): each entry has an `id`, a driver `kind` (e.g. `telegram`, `discord`, `file`), whether it's `enabled`, and kind-specific non-secret `settings`. Secrets (bot tokens, API keys, etc.) are never stored in YAML - they are read from environment variables named `SHARDS3_BACKEND_<ID>_<KEY>` (id and key upper-cased), so a backend's non-secret configuration and its secrets are managed separately. Disabled backends are skipped entirely at startup, including their secret lookups.

New backend kinds register themselves with the storage layer via an `init()` call (`interfaces.RegisterKind`); adding one does not require touching existing backend code.

## Metadata
Metadata is stored in a simple SQLite Database (./filepath). The metadata contains information  
about each S3-Object as well as information about each **chunk** and **shard**

## Chunking and Sharding

## Example
The following diagram shows how an S3-Objects is split and stored:
[![](https://mermaid.ink/img/pako:eNpNk2uTqjgQhv9KKp_OqZIZ7iBVe1HxNorjgMyMrltWhAgRCRiCiHPmvy_M5dTmU9Jdz9ud6rffYJCFGFrwcMqqIEaMg5W9ZVsKmtP7x1PA4_6IA_4vEIQ_wS8vPxEOCOUZkGTTIX0QxCVNil-g_w31f3wSYNBmfv6fwyiIP4FPiRBxBBANQY4Y4TUomvphozX41hr8GNK2vxB4H6kvNZsUnJF9yfEXAlCa0QjsUZBg2ir8_oL99xsgaWSBLYw5zwvr_j7FLEUkvCM0uW9S9zlKMgsv2Mg_Spfo1nvaeZeX-UtyIuNK6NG4cG4bL6_055NYqr0eGyer0dk33d1cGyQ-8svXkC26Xb3bNXVdl0c0iKYPl9JkZ-0cosPmeYoCujZVD62ueFbskcjq7nohubNugnUNlfF07umH7IFLnjJgusuIu0iIIl9PczaKFyrZn1VB4jW2F3q1kPtpXj30nw_VanZzp_tiriaydNmYdXSJlZ29RHUuCQY7jvoXczXeac_zSfwkD7KJ_nKdTFJSj8-BJyQpX4-MV2mmRq5SrK_yeey_Loca99dNyxvx6p_qCV1M6Goy6ct8VleFTnb-8WISQSb6NRs5Tjnsaxv_FtJHge9osUkfn5b72rZnzmlV-cuijqdDc3yrp87KG0zUJAmvPD7HkROY3lOwXJYbV7CnD-5-PnOPaxd5pMbLaKiJinOu_uJ1jv_IabSFHZBnRTtC3t6DjDbjR42D2lB2OGwheIcdGDESQouzEnfg14wbX7-1PmjIGKd4C1siRCzZwi1tmRzRTZal3xjLyiiG1gGdiuZV5o1BsU1QxFD6O8oah2E2yErKoWWohtGBOCQ8Y87nIn3s04cytN7gFVqSLt4ZhqiZqiTqqqopHVhDS5AU466riKYuq7JuaIb53oG3j17EO9NUFEkRRVGVZElVuu__AYEfMNw?type=png)](https://mermaid.live/edit#pako:eNpNk2uTqjgQhv9KKp_OqZIZ7iBVe1HxNorjgMyMrltWhAgRCRiCiHPmvy_M5dTmU9Jdz9ud6rffYJCFGFrwcMqqIEaMg5W9ZVsKmtP7x1PA4_6IA_4vEIQ_wS8vPxEOCOUZkGTTIX0QxCVNil-g_w31f3wSYNBmfv6fwyiIP4FPiRBxBBANQY4Y4TUomvphozX41hr8GNK2vxB4H6kvNZsUnJF9yfEXAlCa0QjsUZBg2ir8_oL99xsgaWSBLYw5zwvr_j7FLEUkvCM0uW9S9zlKMgsv2Mg_Spfo1nvaeZeX-UtyIuNK6NG4cG4bL6_055NYqr0eGyer0dk33d1cGyQ-8svXkC26Xb3bNXVdl0c0iKYPl9JkZ-0cosPmeYoCujZVD62ueFbskcjq7nohubNugnUNlfF07umH7IFLnjJgusuIu0iIIl9PczaKFyrZn1VB4jW2F3q1kPtpXj30nw_VanZzp_tiriaydNmYdXSJlZ29RHUuCQY7jvoXczXeac_zSfwkD7KJ_nKdTFJSj8-BJyQpX4-MV2mmRq5SrK_yeey_Loca99dNyxvx6p_qCV1M6Goy6ct8VleFTnb-8WISQSb6NRs5Tjnsaxv_FtJHge9osUkfn5b72rZnzmlV-cuijqdDc3yrp87KG0zUJAmvPD7HkROY3lOwXJYbV7CnD-5-PnOPaxd5pMbLaKiJinOu_uJ1jv_IabSFHZBnRTtC3t6DjDbjR42D2lB2OGwheIcdGDESQouzEnfg14wbX7-1PmjIGKd4C1siRCzZwi1tmRzRTZal3xjLyiiG1gGdiuZV5o1BsU1QxFD6O8oah2E2yErKoWWohtKBOCQ8Y87nIn3s04cytN7gFVqSLt4ZhqiZqiTqqqo1QA0tQVKMu64imrqsyrqhGeZ7B94-ehHvTFNRJEUURVWSJVXpvv8Hf4sw2A)
While this looks pretty good, it can be optimized further by merging the shards into larger blobs up the the max file size of each backend. This reduces the number of api calls for each backend.  
**Final Result:**
[![](https://mermaid.ink/img/pako:eNqVUrFugzAQ_RXrlqQSicAGDB7TjI3UqUPFYsIBVgBHxqhNo_x7gUKiqhnSzXfv3XvPujvDXmcIAlarVWKSZq-bXBVieBJiDWItj1NFSFvqjzdZddgKksuqxRlIoLR19SJTrNoEfoG2xBoFWeTaYGsXQ3OymtRXKVp5E9rI_QGbjHiCUJKrCluH8GCnNsRqK6sEZupEr3RKlr67U-nT4Oy793AWzDgL_lrRvv2A1VZaSZb0KkWD_-Gv0ih7us-4hWGCeGOYB77Nr0o8AAcKozIQ1nToQI2mlkMJ52EugXEPAzWBTJpDL9hc-pmjbN61rucxo7uiBDHuz4HumEmLWyULI-tr1_Qx0TzrrrEg4sB3ADNltdn9HNJ4T6MwiDN8gvDcNY1ozDxGeeTFEe0nTn2bxuswpCyMOPd9z3fDiwNfYxZ33bPd0I38KOacM8Yu33GAxdg?type=png)](https://mermaid.live/edit#pako:eNqVUstugzAQ_BVrL0klEhmbp49pjo3UUw8VFxMWsAI4MkZtGuXfCxQSVc0hvXl3ZmfG2j3DXmcIAlarVWKSZq-bXBVieBJiDWItj1NFSFvqjzdZddgKksuqxRlIoLR19SJTrNoEfoG2xBoFWeTaYGsXQ3OymtRXKVp5E9rI_QGbjLiCMJKrCluHhP5ObYjVVlYJzNSJXumULD26U-nT4OzRezj3Z5z7f61Y337AaiutJEt2lWL-__BXaZQ93WfcwnBB3DHMA98Or0qhDw4URmUgrOnQgRpNLYcSzsNcAuMeBmoCmTSHXrC59DNH2bxrXc9jRndFCWLcnwPdMZMWt0oWRtbXruljonnWXWNBxJQ7gJmy2ux-Dmm8p1EYxBk-QbhBuGYRi1nkBUEYB1HkwKlvu2wdBIxHHuWcUe6yiwNfYxa6jrlLAxp5URyGIef88g1xssXS)
In this example there are three backends:
- Backend 1. Allows file size of up to 40MiB
- Backend 2. Allows file size of up to 25MiB
- Backend 3. Allows file size of up to 200Mib

Since the smallest allowed size if 25MiB, each encoded shard must be 25MiB.  The number of allowed backends to fail is one, which is why there are three parity shards. If any of the backends fail, the data is still available. If more than one backend fails, the data is not recoverable.