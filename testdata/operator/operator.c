#include <stdio.h>
#include <string.h>

void f() {
	double a;
	float b;
	if (a < b) {
		printf("OK!\n");
	}
}

typedef struct {
    char msg[10];
} foo_t;

int main() {
    (void)0;
    foo_t foo = {"Hi, c2go!"};
    foo_t *pfoo = &foo;
    char msg[] = {'a', 'b', '\0'};
    char *pmsg = msg;
    printf("%c\n", msg[1]);
    pmsg[1] = (msg[0]>='a'?'!':'?'), printf("%s\n", pmsg),
    pfoo->msg[0] += 'a'-'A',
    pfoo->msg[2] = '!', printf("%s\n", foo.msg);
    return 0;
}
