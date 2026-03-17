#include <cuda_runtime.h>

static const int GPU_MAX_DEVICES = 8;

extern "C" int gpu_device_count(void) {
    int n = 0;
    cudaGetDeviceCount(&n);
    return n;
}

extern "C" int gpu_max_devices(void) {
    return GPU_MAX_DEVICES;
}
