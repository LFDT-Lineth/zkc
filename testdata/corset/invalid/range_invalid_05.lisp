;;error:4:13-20:malformed column
(defcolumns (X :u16) (Y :u16))

(definrange (- X Y) 16)
