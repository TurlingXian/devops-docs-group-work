#include <stdio.h>

int fai(int *t){
	int r = *t;
	// printf("Value of t before: %d\n", *t);
	*t++;
	// printf("Value of t after: %d\n", *t);
	// printf("Return value: %d\n", r);
	return(r);
}

int k = 1;
int t = 1;

int ticket[3] = {0, 0, 0};

int main(){
	for(int i = 0; i < 3; i++){		
		ticket[i] = fai(&k);
		printf("Value of ticket[%d]: %d\n",i , k);
	}
	for(int i = 0; i < 3; i++){
		if(ticket[i] == t){
			printf("Cirtical selection for %d.\n", i);
			fai(&t);
		}
	}

	return 0;
}
