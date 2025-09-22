/*
 * Main source code file for lsh shell program
 *
 * You are free to add functions to this file.
 * If you want to add functions in a separate file(s)
 * you will need to modify the CMakeLists.txt to compile
 * your additional file(s).
 *
 * Add appropriate comments in your code to make it
 * easier for us while grading your assignment.
 *
 * Using assert statements in your code is a great way to catch errors early and make debugging easier.
 * Think of them as mini self-checks that ensure your program behaves as expected.
 * By setting up these guardrails, you're creating a more robust and maintainable solution.
 * So go ahead, sprinkle some asserts in your code; they're your friends in disguise!
 *
 * All the best!
 */
#include <assert.h>
#include <ctype.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <readline/readline.h>
#include <readline/history.h>

// The <unistd.h> header is your gateway to the OS's process management facilities.
#include <unistd.h>

// Allow the used of several system-related calls
#include <sys/types.h>
#include <sys/wait.h>
#include<signal.h>

#include "parse.h"

#define MAX_LEN 256

static void print_cmd(Command *cmd);
static void print_pgm(Pgm *p);
void stripwhite(char *);
void cd_builtin(char *path);
void exit_builtin(char* line);

pid_t fg_pid;

typedef struct job_t {
  pid_t pid;
  Pgm* pgm;
  int status;
} job;

job* job_table[MAX_LEN + 1];
int job_count = 0;

job* create_job(pid_t job_pid, Pgm* pgm){
  if (job_count == MAX_LEN) {
    printf("Job table full!\n");
    return NULL;
  }

  job* new_job = malloc(sizeof(job));
  if (!new_job) {
    perror("malloc failed");
    return NULL;
  }

  new_job->pid = job_pid;
  new_job->pgm = pgm;
  new_job->status = 0;

  job_table[job_count++] = new_job;
  return new_job;
}

void print_job(job *j) {
    if (!j || !j->pgm) return;

    printf("Job pid: %d, status: %d\n", j->pid, j->status);

    Pgm *p = j->pgm;
    print_pgm(p);
}

void signal_child_handler(int singal_number){
  int child_status;
  pid_t child_pid;

  while ((child_pid = waitpid(-1, &child_status, WNOHANG)) > 0){ // child status get ok
    for(int i = 0; i < job_count; i++){
      if (job_table[i]->pid == child_pid){
        job_table[i]->status = 2; // exit (success)
        printf("\n[BG] Job is finished, pid: %d\n", child_pid);
        break;
      }
    }
  }
}

void foreground_sig_handler(int signal_number){
    if (fg_pid > 0) {
        kill(fg_pid, SIGINT);
        printf("\n[FG] Process %d terminated by Ctrl+C\n", fg_pid);
        fg_pid = 0;  // reset
    } else {
        printf("\n");  // no foreground job, just print newline
        fflush(stdout); // make sure newline shows immediately
    }
}

int main(void){ 
  // declare a command to process later, depended on background or not
  Command to_process;
  signal(SIGCHLD, signal_child_handler);
  signal(SIGINT, foreground_sig_handler);

  // allocated the 0 position for this shell

  // debug message
  
  for (;;){
    char *line;
    line = readline("> ");

    // debug message
    // printf("lsh shell process ID: %d\n", lsh_jobs.pid);
    // printf("lsh invoked command: %s\n", lsh_jobs.cmd);

    // handle EOF - Ctrl + D signal
    if(line == NULL){
      exit_builtin(line);
    }

    // Remove leading and trailing whitespace from the line
    stripwhite(line);

    // If stripped line not blank
    if (*line)
    {
      add_history(line);

      Command cmd;
      if (parse(line, &cmd) == 1)
      {
        // Just prints cmd
        // print_cmd(&cmd);
        to_process = cmd;
      }
      else
      {
        printf("Parse ERROR\n");
      }
    }

    // begin to process the command
    Pgm *p = to_process.pgm;
    while (p != NULL){
      // printf("Current command to execute: %s\n", *(p->pgmlist));

      // handle exit first
      if (strcmp(*(p->pgmlist), "exit") == 0){
        exit_builtin(line);
      }

      if (strcmp(*(p->pgmlist), "cd") == 0) {
        cd_builtin(p->pgmlist[1]);
        p = p->next;
        continue;
      }

      pid_t pid = fork();
      
      if (pid < 0){
        perror("Fork failed");
        free(line);
        return -1;
      }

      if(pid == 0){
        // child process handler
        if(execvp(*(p->pgmlist), p->pgmlist) == -1){
          perror("lsh, exec failed");
        };
      }

      else{
        // handle with parent, add the child to the list and tracking its progress
        // should point to the BEGINNING of the array pgmlist
        job* new_job = create_job(pid, p);
        // debug
        // print_job(new_job);
        if(to_process.background == 1){
          printf("[BG] Job started: pid=%d, cmd=", pid);
          print_pgm(p);
        }
        else{
          //foreground process
          fg_pid = pid;
          waitpid(pid, NULL, 0);
          new_job->status=2; // and manually marked it as completed
          // fg_pid = 0;
        }
      }

      p = p->next;
    }
    

    // Clear memory
    free(line);
  }
  
  return 0;
}

/*
 * Print a Command structure as returned by parse on stdout.
 *
 * Helper function, no need to change. Might be useful to study as inspiration.
 */
static void print_cmd(Command *cmd_list)
{
  printf("------------------------------\n");
  printf("Parse OK\n");
  printf("stdin:      %s\n", cmd_list->rstdin ? cmd_list->rstdin : "<none>");
  printf("stdout:     %s\n", cmd_list->rstdout ? cmd_list->rstdout : "<none>");
  printf("background: %s\n", cmd_list->background ? "true" : "false");
  printf("Pgms:\n");
  print_pgm(cmd_list->pgm);
  printf("------------------------------\n");
}

/* Print a (linked) list of Pgm:s.
 *
 * Helper function, no need to change. Might be useful to study as inpsiration.
 */
static void print_pgm(Pgm *p)
{
  if (p == NULL)
  {
    return;
  }
  else
  {
    char **pl = p->pgmlist;

    /* The list is in reversed order so print
     * it reversed to get right
     */
    print_pgm(p->next);
    printf("            * [ ");
    while (*pl)
    {
      printf("%s ", *pl++);
    }
    printf("]\n");
  }
}


/* Strip whitespace from the start and end of a string.
 *
 * Helper function, no need to change.
 */
void stripwhite(char *string)
{
  size_t i = 0;

  while (isspace(string[i]))
  {
    i++;
  }

  if (i)
  {
    memmove(string, string + i, strlen(string + i) + 1);
  }

  i = strlen(string) - 1;
  while (i > 0 && isspace(string[i]))
  {
    i--;
  }

  string[++i] = '\0';
}

/**
 * Handle the built-in cd command to change the current working directory.
 * If path is NULL, change to the home directory.
 * If path is invalid, print an error message.
 * @param path The target directory path.
 * @return void (the working directory is changed by chdir() command, can use getcwd() to verify)
 */
void cd_builtin(char *path) {
    // if no argument, go to home directory
    // debug message to make sure this shell used the buitl-in, not the executable from PATH
    // printf("Using the built-in cd command\n");
    if (path == NULL) {
      char *home_dir = getenv("HOME");
      if (home_dir != NULL) {
        if (chdir(home_dir) != 0) {
          perror("chdir to HOME failed");
        }
      } 
      else {
        fprintf(stderr, "HOME environment variable not set.\n");
      }
    } 
    
    else {
      // go to the passed directory
          if (chdir(path) != 0)
            perror("chdir failed");
    }
  }

/**
 * Exit handler for the built-in exit command.
 * This command can be achieved by two means: EOF signal (Crtl + D) of exit itself in the shell.
 * @param: A character pointer to the output that the shell is currently handeling.
 * @return: void (the program will terminate in the main function after calling this)
 */
void exit_builtin(char* line){ 
    printf("Exit command detected, terminating the shell.\n");
    free(line);
    exit(0);
  }