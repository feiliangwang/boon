#include <cuda_runtime.h>

extern "C" int gpu_device_count(void) {
    int n = 0;
    cudaGetDeviceCount(&n);
    return n;
}
