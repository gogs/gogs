package repo

import (
	"fmt"
	"strings"
)

const advertise_refs = "--advertise-refs"

func command(cmd string, opts ...string) string {
	return fmt.Sprintf("git %s %s", cmd, strings.Join(opts, " "))
}

/*func upload_pack(repository_path string, opts ...string) string {
	cmd = "upload-pack"
	opts = append(opts, "--stateless-rpc", repository_path)
	return command(cmd, opts...)
}

func receive_pack(repository_path string, opts ...string) string {
	cmd = "receive-pack"
	opts = append(opts, "--stateless-rpc", repository_path)
	return command(cmd, opts...)
}*/

/*func update_server_info(repository_path, opts = {}, &block)
      cmd = "update-server-info"
      args = []
      opts.each {|k,v| args << command_options[k] if command_options.has_key?(k) }
      opts[:args] = args
      Dir.chdir(repository_path) do # "git update-server-info" does not take a parameter to specify the repository, so set the working directory to the repository
        self.command(cmd, opts, &block)
      end
    end

    def get_config_setting(repository_path, key)
      path = get_config_location(repository_path)
      raise "Config file could not be found for repository in #{repository_path}." unless path
      self.command("config", {:args => ["-f #{path}", key]}).chomp
    end

    def get_config_location(repository_path)
      non_bare = File.join(repository_path,'.git') # This is where the config file will be if the repository is non-bare
      if File.exists?(non_bare) then # The repository is non-bare
        non_bare_config = File.join(non_bare, 'config')
        return non_bare_config if File.exists?(non_bare_config)
      else # We are dealing with a bare repository
        bare_config = File.join(repository_path, "config")
        return bare_config if File.exists?(bare_config)
      end
      return nil
    end

  end
*/
