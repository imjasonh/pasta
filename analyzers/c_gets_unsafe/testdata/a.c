#include <stdio.h>

int main(void) {
    char buf[10];
    gets(buf);                      // want "gets() is unsafe"
    fgets(buf, sizeof(buf), stdin); // OK
    return 0;
}
