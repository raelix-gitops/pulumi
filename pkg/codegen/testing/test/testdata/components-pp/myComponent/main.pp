config input string {
}

resource random_pet "random:index/randomPet:RandomPet" {
  prefix = input
}

output result {
    value = random_pet.result
}