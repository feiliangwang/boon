/*
 * gpu_batch.cu – Batch TRON address derivation pipeline on CUDA
 *
 * Reuses the shared device helpers from gpu_enumerate.cu while keeping the
 * batch kernel in its own translation unit, so batch/enumerate register usage
 * remains isolated and multi-GPU host ABI stays unchanged.
 */

#define GPU_ENUMERATE_SHARED_ONLY
#include "gpu_enumerate.cu"

/* ================================================================
 * Kernel: one thread per mnemonic
 * ================================================================ */
__global__ void tron_batch_kernel(
    const uint8_t *mdata,
    const int     *moff,
    const int     *mlen,
    int            count,
    uint8_t       *out)
{
    int idx = blockIdx.x * blockDim.x + threadIdx.x;
    if (idx >= count) return;

    const uint8_t *mn = mdata + moff[idx];
    uint32_t       ml = (uint32_t)mlen[idx];

    /* Match enumerate's scratch reuse so both entry points share the same
     * crypto implementation and a very similar stack profile. */
    uint8_t tmp[64];
    uint8_t pub[64];
    en_pbkdf2_hmac_sha512(mn, ml, tmp);
    en_bip44_tron(tmp, tmp);
    en_priv_to_upub64(tmp, pub);
    en_keccak256(pub, 64, tmp);

    memcpy(out + (int64_t)idx * 20, tmp + 12, 20);
}

/* ================================================================
 * Host functions
 * ================================================================ */
extern "C" int gpu_compute_addresses(
    int            device_id,
    const uint8_t *mnemonic_data,
    const int     *mnemonic_offsets,
    const int     *mnemonic_lengths,
    int            count,
    uint8_t       *addresses_out)
{
    if (cudaSetDevice(device_id) != cudaSuccess) return -1;
    if (count <= 0) return 0;

    int max_data = 0;
    for (int i = 0; i < count; i++) {
        int end = mnemonic_offsets[i] + mnemonic_lengths[i];
        if (end > max_data) max_data = end;
    }
    if (max_data <= 0) max_data = 1;

    uint8_t *d_data = NULL;
    int     *d_off  = NULL;
    int     *d_len  = NULL;
    uint8_t *d_out  = NULL;
    if (cudaMalloc(&d_data, (size_t)max_data)            != cudaSuccess) goto err;
    if (cudaMalloc(&d_off,  (size_t)count * sizeof(int)) != cudaSuccess) goto err;
    if (cudaMalloc(&d_len,  (size_t)count * sizeof(int)) != cudaSuccess) goto err;
    if (cudaMalloc(&d_out,  (size_t)count * 20)          != cudaSuccess) goto err;

    cudaMemcpy(d_data, mnemonic_data,    (size_t)max_data,            cudaMemcpyHostToDevice);
    cudaMemcpy(d_off,  mnemonic_offsets, (size_t)count * sizeof(int), cudaMemcpyHostToDevice);
    cudaMemcpy(d_len,  mnemonic_lengths, (size_t)count * sizeof(int), cudaMemcpyHostToDevice);

    /* Default CUDA thread stack is 1024 bytes; the shared crypto pipeline
     * needs the same stack headroom as enumerate. */
    cudaDeviceSetLimit(cudaLimitStackSize, 65536);
    {
        int bs = 32;
        int nb = (count + bs - 1) / bs;
        tron_batch_kernel<<<nb, bs>>>(d_data, d_off, d_len, count, d_out);
        if (cudaDeviceSynchronize() != cudaSuccess) goto err;
    }

    cudaMemcpy(addresses_out, d_out, (size_t)count * 20, cudaMemcpyDeviceToHost);
    cudaFree(d_data);
    cudaFree(d_off);
    cudaFree(d_len);
    cudaFree(d_out);
    return 0;

err:
    if (d_data) cudaFree(d_data);
    if (d_off)  cudaFree(d_off);
    if (d_len)  cudaFree(d_len);
    if (d_out)  cudaFree(d_out);
    return -1;
}
