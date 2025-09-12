#include <dirent.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

# define MAX_LEN 256

// additional process for printing the dir_stream type
// first, pick the 3 most common types: directory, file and symlink

/**
 * Additional process for printing the dir_stream type.
 * First, pick the 3 most common types: directory, file and symlink.
 * DI_DIR = directory = 4
 * DI_LNK = link = 10
 * DI_REG = regular file = 8
 *
 * @c: The input, which is a unsigned character, as documentation.
 *
 * @return: The letter to represent the directory type.
 */
char additional_dir_type_process(unsigned char c){
    char ret;
    switch (c)
    {
    case 4:
        ret = 'd';
        break;
    case 10:
        ret = 'l';
        break;
    default:
        ret = '-';
        break;
    }
    return ret;
}

/**
 * Collect the result into an array, sort them to make they're...more like the actual "ls".
 * This is a struct.
 */
struct ls_entry{
    char name[MAX_LEN + 1];
    unsigned char type;
    unsigned long size;
};

/**
 * For sorting the "directories".
 * Pass two pointer, compare their data. Return int result.
 * @param: Two entry (ls_entry).
 * 
 * @return: indicate which comes first.
 */
int cmp(const void *p1, const void *p2){
    const struct ls_entry *ls_entry_1 = p1;
    const struct ls_entry *ls_entry_2 = p2;

    // place the current directory . first
    if (strcmp(ls_entry_1->name, ".") == 0) return -1;
    if (strcmp(ls_entry_2->name, ".") == 0) return 1;

    // place the upper directory .. second
    if (strcmp(ls_entry_1->name, "..") == 0) return -1;
    if (strcmp(ls_entry_2->name, "..") == 0) return 1;

    return strcmp(ls_entry_1->name, ls_entry_2->name);
}

int main(int arc, char* argv[]){
    // a pointer points to a directory
    DIR *dir_stream;
    struct dirent *dir_read; // hold the "current result of dir stream"
    struct ls_entry *entries = NULL;
    size_t size_of_entries = 0;

    // need to open it first before displaying content?
    // the default behavior or ls is diplaying current "." directory
    dir_stream = opendir(".");
    
    // error handler
    if (dir_stream == NULL){
        perror("Error while trying to open directory");
        return -1;
    }

    while((dir_read = readdir(dir_stream)) != NULL){
        // allocate a (reallocate in fact) space for incoming entry
        entries = realloc(entries, (size_of_entries + 1)*sizeof(struct ls_entry));
        strcpy(entries[size_of_entries].name, dir_read->d_name);
        entries[size_of_entries].size = dir_read->d_reclen;
        entries[size_of_entries].type = additional_dir_type_process(dir_read->d_type);
        size_of_entries++;
    }
    // if the command readdir (read directory) from the dir_stream we open is not NULL
    // first, we should grab the basic ones, file size and file name?
    // while ((dir_read = readdir(dir_stream)) != NULL) {
    //     printf("%c\t%s\t%d bytes\n", additional_dir_type_process(dir_read->d_type), dir_read->d_name, dir_read->d_reclen);
    // }

    if(closedir(dir_stream) == -1){
        perror("Cannot close file stream, bugs happened");
        return -1;
    }

    // sort the whole result array
    qsort(entries, size_of_entries, sizeof(struct ls_entry), cmp);

    // normally, we should hide the "." and ".." or any directory begin with .
    // the most primitive of using "hidden"
    for(int i = 2; i < size_of_entries; i++){
        printf("%c\t%s\t%ld bytes\n", entries[i].type, entries[i].name, entries[i].size);
    }

    free(entries);

    return 0;
}