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
#include <signal.h>
#include <termios.h>

#include "parse.h"

#define MAX_LEN 256

static void print_cmd(Command *cmd);
static void print_pgm(Pgm *p);
void stripwhite(char *);
void cd_builtin(char *path);
void exit_builtin(char *line);

void fg_handler();
void child_handler();

typedef struct t_job{
  pid_t job_pid;
} job;

job background_jobs[MAX_LEN];
int background_jobs_count;

pid_t fg_pid;



int main(void)
{
  background_jobs_count = 0;
  signal(SIGINT, fg_handler);
  signal(SIGCHLD, child_handler);

  for (;;)
  {
    char *line = readline("> ");

    // handle EOF - Ctrl + D signal
    if (line == NULL)
    {
      exit_builtin(line);
    }

    stripwhite(line);

    Command to_process;
    memset(&to_process, 0, sizeof(Command));

    if (*line)
    {
      add_history(line);

      Command cmd;
      if (parse(line, &cmd) == 1)
      {
        to_process = cmd;
      }
      else
      {
        printf("Parse ERROR\n");
        free(line);
        continue;
      }
    }

    // begin to process the command
    Pgm *p = to_process.pgm;

    while (p != NULL)
    {
      if (strcmp(*(p->pgmlist), "exit") == 0)
      {
        exit_builtin(line);
      }

      if (strcmp(*(p->pgmlist), "cd") == 0)
      {
        cd_builtin(p->pgmlist[1]);
        p = p->next;
        continue;
      }

      pid_t pid = fork();

      if (pid < 0)
      {
        perror("Fork failed");
        free(line);
        break;
      }

      if (pid == 0){ // child

        execvp(*(p->pgmlist), p->pgmlist);
        perror("lsh, exec failed");
        _exit(1);
      }

      else{ // parent
        if(to_process.background == 1){
          if(background_jobs_count > MAX_LEN){
            perror("Allocated failed for new BG job.\n");
            continue;
          }
          
          job current_bg = {0};
          current_bg.job_pid = pid;

          background_jobs[background_jobs_count] = current_bg; 
          background_jobs_count += 1;

          printf("[BG] Job started with pid %d, cmd= ", pid);
          print_pgm(p);
        }
        else{
          fg_pid = pid;
          int execution_status;
          waitpid(pid, &execution_status, 0);
        }
      }

      p = p->next;
    }

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
void cd_builtin(char *path)
{
  // if no argument, go to home directory
  // debug message to make sure this shell used the buitl-in, not the executable from PATH
  // printf("Using the built-in cd command\n");
  if (path == NULL)
  {
    char *home_dir = getenv("HOME");
    if (home_dir != NULL)
    {
      if (chdir(home_dir) != 0)
      {
        perror("chdir to HOME failed");
      }
    }
    else
    {
      fprintf(stderr, "HOME environment variable not set.\n");
    }
  }

  else
  {
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
void exit_builtin(char *line)
{
  free(line);
  exit(0);
}

void fg_handler(){
  // printf("Caught signal for process %d\n", fg_pid);
  if(fg_pid > 0){
    kill(fg_pid, SIGINT);
    printf("[FG] Process %d terminated by Ctrl+C.\n", fg_pid);
    fg_pid = 0;
  }
  else{
    printf("\n");
    fflush(stdin);
  }
}

void child_handler(){
  int child_status;
  pid_t child_pid;

  while((child_pid = waitpid(-1, &child_status, WNOHANG)) > 0){
    for(int i = 0; i < background_jobs_count; i++){
      if(background_jobs[i].job_pid == child_pid){
        for(int j = i; j < background_jobs_count - 1; j++){
          background_jobs[j] = background_jobs[j + 1];
        }
        background_jobs_count -= 1;
        break;
      }
    }
    printf("\n[BG] Process %d finished.\n", child_pid);
    fflush(stdin);

    rl_on_new_line();
    rl_replace_line("", 0);
    rl_redisplay();
  }
}