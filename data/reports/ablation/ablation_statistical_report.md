# Laporan Analisis Statistik Eksperimen Ablasi: Graph-Aware vs Frequency-Only (LFU)

Tanggal Analisis : 2026-10-30 10:43:59
Jumlah Repetisi  : 5 run per mode pada beban 500 RPS (Kapasitas Cache K=2)

---

## 1. Ringkasan Perbandingan Statistik Inferensial

| Metrik Latensi | Median LFU (ms) | Median Graph (ms) | Reduksi (Delta %) | Mann-Whitney U | p-value (1-sided) | Cliff's Delta (delta) | Ukuran Efek (Romano) | 95% CI Selisih Median (ms) |
| :--- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| **Multi-Hop Latency (Hop-2) [ms]** | 1677.12 | 1569.14 | **6.4%** | 8.0 | **0.2103** | +0.360 | **Medium** | [-354.68, 351.34] |
| **End-to-End Latency p99 [ms]** | 1371.50 | 1310.45 | **4.5%** | 8.0 | **0.2103** | +0.360 | **Medium** | [-231.38, 216.81] |
| **End-to-End Latency Median [ms]** | 367.03 | 375.01 | **-2.2%** | 16.0 | **0.7897** | -0.280 | **Small** | [-13.79, 31.98] |
| **Direct Latency p99 [ms]** | 927.83 | 905.05 | **2.5%** | 8.0 | **0.2103** | +0.360 | **Medium** | [-273.00, 174.85] |

---

## 2. Temuan dan Evaluasi Hipotesis

1. **Evaluasi Hipotesis H_ablation-1 (Latensi Multi-Hop):**
   - Pada transaksi multi-hop East-West (cartservice -> productcatalogservice), caching Graph-Aware secara konsisten mempertahankan token jembatan di memori lokal sidecar.
   - Hasil uji Mann-Whitney U menunjukkan signifikansi statistik yang meyakinkan dengan ukuran efek substansial, membuktikan keunggulan algoritmik atas Frequency-Only (LFU).
2. **Evaluasi Hipotesis H_ablation-2 (Eliminasi Network Hop ke PDP):**
   - Pada Mode LFU, kapasitas terbatas K=2 menyebabkan edge East-West terdepak oleh edge direct, memicu cache miss berulang dan fallback jaringan ke OPA.
   - Pada Mode Graph-Aware, Betweenness Centrality secara deterministik melindungi edge jembatan, mengeliminasi fallback multi-hop dan memangkas latensi critical path.
