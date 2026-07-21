# ShardS3
ShardS3 is a storage service built for backing up data via Amazon S3 API. It uses **multiple storage backends** to store objects.  
Data is split among multiple backends **similar to RAID** using reed-solomon encoding. This allows for **faster up and downloads** and most importantly **adds redundancy** in case a backend fails or data gets deleted.

> **Disclaimer:**
> This is only a fun project. Using services like Discord 
> for backup and data storage may be against their 
> policies.  
> A violation of these policies could lead to 
> consequences. Including banned accounts,
> blocked phone numbers, data loss or other consequences.  
> Use this software at your own risk.

## Storage
The following diagram shows how an S3-Objects is split and stored:

[![](https://mermaid.ink/img/pako:eNpNk21zojoUx79KJq92Z7QFUUBm7t4rokKtaKHW6rrDBAhPQhJDaKvdfveLfdjZvErOOb9_Tib_8wojGmNowKSkz1GGuAD31p7vCWjX6KevgGVY4Ej8At3uD_DbZ2UuQE4EBXJPX-QmiLKGHOrfwPyCzG8fBBhfMt__5jCKsg_gQyJGAgFEYsAQz8UJ1O39cas1_tIaf5uQS38x8N9Tn2pWXgueh43AnwhAFSUpCFF0wOSi8OcJ1n-vIK9SA-xhJgSrjevrCvMK5fFVTg7XbeqaoQM1sMunaz5t0vPoLvAeecm3vncbmqP7U2_mNWzdx_OB5zSpc_Oyc87IXRyDqVT4x-n8MV6SCncDjHtJbG9MMX523K1XVLdV8CiEsmaemhR2ae9sU-FZwedPT938IK_IZhuG1lGdWOVaKe5XzXxenboTW_XrE3NJ_FLwY-BKDjmullozm6-mqmuT2ZCFu9Sh0S0eOxv1xo59bVM_PA-LrPQV3Q8o4kHwhBdoHujEfWJhaHt8nnp3Dacj7UFejbzsZNaRbx0XatYrw61SL82Qi8X0QbOtOy10u7Xr4V2tyLt0p_bsyTiYD617Woc-TcqZkoxEdOj2wrNCq1lkrqdTTR0gEg2aJiycWbIdraTJ4TSRWLko3fM0EvUqLXrU2dwtBuqNN6owS1yHNc72eRlJY9Ytj9jRy-LFPeSnpaPY-qhIlbOT_itODP_DSLqHHcBofflHcdlHlLQeQK2NLiGaJHsI3mAHpjyPoSF4gzvw86Nbc79ezNCSGa7wHl6IGPHDHu7JhWGI7CitvjBOmzSDRoLKuj01rHUptnKUclT9ifLWZpiPaUMENLS-rHUgjnNB-eJjmt6H6l0ZGq_wBRryQL3SNGmg6IoiSZou9zrwBI3hlaQOZbmv6gNpoA-G_bcOPL_3Il3pbancFkt9uSf3leHb_w0AM8g?type=png)](https://mermaid.live/edit#pako:eNpNk2tvozoQhv-K5U-7UtISaLhJZ8-B3KBpSApN02SzQgbMLWATY9qSbv_7IWm7Wn-yZ_y8M9a8foMhjTDUYVzQlzBFjIOH8Z7tCeiW8dOTwDLIcch_gX7_B_jtVUXGQUY4BQNRXWQmCNOGHOrfwPyCzG8fBBidM9__5jAK0w_gQyJCHAFEIlAhlvEW1F39qNMafWmNvk3Iub8IeJfUp9o4qznLgobjTwSgkpIEBCg8YHJW-POE8X9vICsTHexhynlV69fXJWYlyqKrjByuu9R1hQ5Uxw6brtm0SU7Gve8-sYJtPfcuMI2HVpy5TbW-wfOhazeJffu6s0_IWRz9qZB7x-n8KVqSEvd9jMU4sjYmH73YztbNy7vSf-JcWleuHOdWYe0sU2JpzubPz_3sMFiRzTYIxkd5Mi7WUv6waubzsu1PLNmr28oh0WvOjr4j2OS4WirNbL6ayo5FZloV7BKbhnd4ZG_kWyvylE39-KLlaeFJqudTxHz_GS_Q3FeJ81wFgeWyeeLeN4wayuNgZbhpa9ahNz4u5FQsgq1UL82A8cX0UbHG90rg9GvHxbtaGuySnSxak5E_18YPtA48GhczKTZ4eOiLwUmi5Sw019OpIg8RCYdNE-T2LN4aK2FyaCdCVSwK5zQNeb1KcpHam_vFUL51jRJXsWNXjb19WYbCqOoXR2yrRf7qHLJ2aUuWauSJdLKTf3lb4X8qkuxhD1S0Ps-Rn_chJZ0HUGejc4jG8R6C94s7YA8mLIugzlmDe_Bz2p3D386O6PAUl3gPz1iE2GEP9-S9YypEdpSWXxijTZJCPUZF3Z2aqrMqHmcoYaj8E2Wd1zAb0YZwqCs3gtaDOMo4ZYuPL3X5WRdlqL_BV6gPhvKVoghDSZUkQVDUgdiDLdS1K0HWBoMbWR0KQ3Wo3bz34OnSi3CldlcHoqbJotAVUKT3_wGFaDS5)
While this looks pretty good, it can be optimized further by merging the shards into larger blobs up the the max file size of each backend. This reduces the number of api call for each backend.  
**Final Result:**
[![](https://mermaid.ink/img/pako:eNqVUstugzAQ_BVrL0klE2Fsh8THNMdG6qmHiosJy0MBHBmjNo3y7wUCiapWanvzzszOrLV7hr1JEBR4nhfZqN6bOi0y1T8JcRax0sexIqTJzduLLltsFEl12eBERJC7qnzSMZZNBF9Il2OFisxSY7Fxsx4co0Z3L0an70YbvT9gnRCmSEDSosSGklDuig1xxukygkk6yksTk7nwd0X80CcL_yeey4nn8ntU0MF_iNpqp8k8uFkF8n_8s7aFO_2suA_DFWHDMJRI_su3e8HVSXKgkNkiAeVsixQqtJXuSzj3fREMe-ilESTaHjrD-tL1HHX9akw1tVnTZjmoYX8U2mOiHW4LnVld3VDbjYn20bS1AyVCRgGTwhm7ux7ScE-DMagzvINiS7EIQ1-uBPOXQkhO4dShYbDwl2sWBJz1IAsuFD6GUfzFasU7YB1K7gspQ3H5BJxRxXY?type=png)](https://mermaid.live/edit#pako:eNqVUstugzAQ_BVrL0kliMAPSHxMc2yknnqouDhhASuAI2PUplH-vUAhUdVIbW_emd2ZsXbPsDcpggTf9xOb1HtTZzqX_ZMQZxErdRwrQprCvL2ossVGkkyVDU5EAoWryie1w7JJ4BvpCqxQkllmLDZu1oOj1aju79Cpm9Ba7Q9YpySUhJJMl9h4JBZbvSbOOFUmMLWO7aXZkTkPtnr30Dvz4B7PxMQz8dOKdvAfrDbKKTKnVykq_sc_K6vd6X7HLQyTJBzCeOSXTws26QgGHuRWpyCdbdGDCm2l-hLO_VwCwxb61gRSZQ-dYH3pZo6qfjWmmsasafMC5LA9D9pjqhxutMqtqq6o7UKifTRt7UDyiHmAqXbGbr_OaLimQRjkGd5BhhFfxHEgljwMIs5FN3Dq0JgugmgVUsrCHgzpxYOPIUqwWC5ZB6xiwQIuRMwvn0D7xQ8)
In this example there are three backends:
- Backend 1. Allows file size of up to 40MiB
- Backend 2. Allows file size of up to 25MiB
- Backend 3. Allows file size of up to 200Mib

Since the smallest allowed size if 25MiB, each encoded shard must be 25MiB.  The number of allowed backends to fail is one, which is why there are three parity shards. If any of the backends fail, the data is still available. If more than one backend fails, the data is not recoverable.

## Helm
ShardS3 can be deployed via Helm to a Kubernetes Cluster.

## Docker/Podman
```
podman compose -f .\container\compose.yaml up
```
## Supported Backends
- Telegram
- Discord

## S3 API Support
The following S3 Endpoint are supported:

- Bucket CRUD
- Object CRUD
- ListObjectsV2
- Multipart uploads
- HEAD requests
- SigV4 authentication
- Presigned URLs

## Documentation
For a detailed explanation of the program you can checkout the [docs](./docs)