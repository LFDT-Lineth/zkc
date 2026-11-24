;;error:3:40-47:malformed target column
(defcolumns (X :u16) (Y :u16))
(deflookup (l1 :unchecked) ((_ Y)) ((_ (- 1 X))))
