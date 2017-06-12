#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <string.h>

#define INTERVAL 50
#define DEF_NUM_LINES 5
#define DEF_DELAY 5

char *msg = "this is a default (sheddableplus)\n";

void printUsage() {
  printf("Delay usage:\n");
  printf("./delay <num lines> [delay (default: 3s)]\n");
  exit(1);
}

int main(int argc, char **argv) {
  int numLines = DEF_NUM_LINES;
  int delay = DEF_DELAY;
  switch (argc) {
    case 3:
      delay = atoi(argv[2]);
    case 2:
      numLines = atoi(argv[1]);
      break;
    case 1:
      break;
    default:
      printUsage();
  }
  if (delay == 0 || numLines == 0) {
    printUsage();
  }
  for (int count = 0; count < numLines; count++) {
    write(1, msg, strlen(msg));
  }
  for (int i = 0; i < strlen(msg); i++) {
    if (msg[i] == '\n') {
      sleep(delay);
    }
    write(1, &msg[i], 1);
  }
  for (int count = 0; count < numLines; count++) {
    write(1, msg, strlen(msg));
  }
  return 0;
}
